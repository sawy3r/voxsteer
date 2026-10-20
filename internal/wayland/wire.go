//go:build linux

package wayland

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

// displayID is the wl_display object every connection starts with.
const displayID = 1

// replyTimeout bounds each wait for the compositor so a stalled compositor
// cannot hang dictation.
const replyTimeout = 5 * time.Second

// message accumulates request arguments in wire format.
type message []byte

func (m *message) uint(value uint32) {
	*m = binary.NativeEndian.AppendUint32(*m, value)
}

func (m *message) string(value string) {
	m.uint(uint32(len(value) + 1))
	*m = append(*m, value...)
	*m = append(*m, make([]byte, 4-len(value)%4)...)
}

// conn is a minimal Wayland client: enough to bind globals, make requests
// and wait for the compositor with wl_display.sync.
type conn struct {
	sock     *net.UnixConn
	next     uint32
	inbound  []byte
	handlers map[uint32]func(opcode uint16, body []byte) error
}

func dial() (*conn, error) {
	path := os.Getenv("WAYLAND_DISPLAY")
	if path == "" {
		path = "wayland-0"
	}
	if !filepath.IsAbs(path) {
		dir := os.Getenv("XDG_RUNTIME_DIR")
		if dir == "" {
			return nil, errors.New("XDG_RUNTIME_DIR is not set")
		}
		path = filepath.Join(dir, path)
	}
	sock, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, err
	}
	c := &conn{sock: sock, next: displayID + 1, handlers: map[uint32]func(uint16, []byte) error{}}
	c.handlers[displayID] = handleDisplay
	return c, nil
}

func (c *conn) close() {
	_ = c.sock.Close()
}

func (c *conn) newID() uint32 {
	id := c.next
	c.next++
	return id
}

// send writes one request, passing fd alongside it when fd is not negative.
func (c *conn) send(object uint32, opcode uint16, body message, fd int) error {
	var request message
	request.uint(object)
	request.uint(uint32(8+len(body))<<16 | uint32(opcode))
	request = append(request, body...)
	if err := c.sock.SetWriteDeadline(time.Now().Add(replyTimeout)); err != nil {
		return err
	}
	if fd >= 0 {
		_, _, err := c.sock.WriteMsgUnix(request, unix.UnixRights(fd), nil)
		return err
	}
	_, err := c.sock.Write(request)
	return err
}

// roundtrip waits until the compositor has processed every request sent so
// far, surfacing any protocol error it raised on the way.
func (c *conn) roundtrip() error {
	callback := c.newID()
	done := false
	c.handlers[callback] = func(opcode uint16, _ []byte) error {
		done = opcode == 0
		return nil
	}
	defer delete(c.handlers, callback)
	var sync message
	sync.uint(callback)
	if err := c.send(displayID, 0, sync, -1); err != nil {
		return err
	}
	for !done {
		if err := c.dispatch(); err != nil {
			return err
		}
	}
	return nil
}

// dispatch reads and handles one event. Events for objects without a handler
// are skipped.
func (c *conn) dispatch() error {
	for {
		if len(c.inbound) >= 8 {
			size := int(binary.NativeEndian.Uint32(c.inbound[4:]) >> 16)
			if size < 8 {
				return fmt.Errorf("malformed event of %d bytes", size)
			}
			if len(c.inbound) >= size {
				object := binary.NativeEndian.Uint32(c.inbound)
				opcode := uint16(binary.NativeEndian.Uint32(c.inbound[4:]))
				body := c.inbound[8:size]
				c.inbound = c.inbound[size:]
				if handler, ok := c.handlers[object]; ok {
					return handler(opcode, body)
				}
				return nil
			}
		}
		if err := c.sock.SetReadDeadline(time.Now().Add(replyTimeout)); err != nil {
			return err
		}
		buffer := make([]byte, 4096)
		n, err := c.sock.Read(buffer)
		if err != nil {
			return fmt.Errorf("read from compositor: %w", err)
		}
		c.inbound = append(c.inbound, buffer[:n]...)
	}
}

func handleDisplay(opcode uint16, body []byte) error {
	if opcode != 0 { // delete_id needs no action: ids are never reused here
		return nil
	}
	args := eventArgs(body)
	object, code := args.uint(), args.uint()
	return fmt.Errorf("compositor error on object %d (code %d): %s", object, code, args.string())
}

// bindGlobals binds the first seat and the virtual keyboard manager.
func (c *conn) bindGlobals() (seat, manager uint32, err error) {
	type global struct{ name, version uint32 }
	globals := map[string]global{}
	registry := c.newID()
	c.handlers[registry] = func(opcode uint16, body []byte) error {
		if opcode == 0 {
			args := eventArgs(body)
			name := args.uint()
			iface := args.string()
			if _, seen := globals[iface]; !seen {
				globals[iface] = global{name: name, version: args.uint()}
			}
		}
		return nil
	}
	var request message
	request.uint(registry)
	if err := c.send(displayID, 1, request, -1); err != nil {
		return 0, 0, err
	}
	if err := c.roundtrip(); err != nil {
		return 0, 0, err
	}
	ids := map[string]uint32{}
	for _, iface := range []string{seatInterface, managerInterface} {
		g, ok := globals[iface]
		if !ok {
			return 0, 0, fmt.Errorf("the compositor does not offer %s", iface)
		}
		id := c.newID()
		var bind message
		bind.uint(g.name)
		bind.string(iface)
		bind.uint(1)
		bind.uint(id)
		if err := c.send(registry, 0, bind, -1); err != nil {
			return 0, 0, err
		}
		ids[iface] = id
	}
	return ids[seatInterface], ids[managerInterface], nil
}

// eventArgs decodes event arguments; a truncated event yields zero values
// rather than a panic.
type eventArgs []byte

func (a *eventArgs) uint() uint32 {
	if len(*a) < 4 {
		*a = nil
		return 0
	}
	value := binary.NativeEndian.Uint32(*a)
	*a = (*a)[4:]
	return value
}

func (a *eventArgs) string() string {
	length := int(a.uint())
	padded := (length + 3) &^ 3
	if length == 0 || len(*a) < padded {
		*a = nil
		return ""
	}
	value := string((*a)[:length-1])
	*a = (*a)[padded:]
	return value
}
