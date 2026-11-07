//go:build desktop && darwin

#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>

#include "_cgo_export.h"

static const CGFloat windowWidth = 460;
static const CGFloat windowHeight = 596;

@interface VoxSteerSettingsWindow
    : NSObject <NSWindowDelegate, WKNavigationDelegate, WKScriptMessageHandlerWithReply>
@property(nonatomic, strong) NSWindow *window;
@property(nonatomic, strong) WKWebView *webView;
@end

@implementation VoxSteerSettingsWindow

- (instancetype)init {
	self = [super init];
	if (self == nil) {
		return nil;
	}
	WKWebViewConfiguration *configuration = [[WKWebViewConfiguration alloc] init];
	[configuration.userContentController addScriptMessageHandlerWithReply:self
	                                                         contentWorld:[WKContentWorld pageWorld]
	                                                                 name:@"voxsteer"];
	NSRect frame = NSMakeRect(0, 0, windowWidth, windowHeight);
	self.webView = [[WKWebView alloc] initWithFrame:frame configuration:configuration];
	self.webView.navigationDelegate = self;

	self.window = [[NSWindow alloc]
	    initWithContentRect:frame
	              styleMask:NSWindowStyleMaskTitled | NSWindowStyleMaskClosable |
	                        NSWindowStyleMaskFullSizeContentView
	                backing:NSBackingStoreBuffered
	                  defer:NO];
	self.window.title = @"VoxSteer Settings";
	// The page's deck runs up under the title bar, so the window reads as one
	// surface. The title stays set for the Window menu and VoiceOver.
	self.window.titlebarAppearsTransparent = YES;
	self.window.titleVisibility = NSWindowTitleHidden;
	self.window.releasedWhenClosed = NO;
	self.window.delegate = self;
	self.window.contentView = self.webView;
	[self.window center];
	return self;
}

- (void)userContentController:(WKUserContentController *)controller
      didReceiveScriptMessage:(WKScriptMessage *)message
                 replyHandler:(void (^)(id, NSString *))replyHandler {
	if (![message.body isKindOfClass:[NSString class]]) {
		replyHandler(nil, @"Settings messages must be strings.");
		return;
	}
	char *reply = settingsWindowRequest((char *)[(NSString *)message.body UTF8String]);
	replyHandler([NSString stringWithUTF8String:reply], nil);
	free(reply);
}

// The page is the only thing this view ever shows. Without this, dropping a file
// onto the window would navigate away from the form.
- (void)webView:(WKWebView *)webView
    decidePolicyForNavigationAction:(WKNavigationAction *)action
                    decisionHandler:(void (^)(WKNavigationActionPolicy))decisionHandler {
	BOOL allowed = [action.request.URL.absoluteString isEqualToString:@"about:blank"];
	decisionHandler(allowed ? WKNavigationActionPolicyAllow : WKNavigationActionPolicyCancel);
}

// VoxSteer has no Dock icon, so hand focus back to the previous application.
- (void)windowWillClose:(NSNotification *)notification {
	[NSApp hide:nil];
}

@end

static VoxSteerSettingsWindow *settingsWindow;

// A menu bar application has no menu of its own, and without one the standard
// editing shortcuts do nothing, which would make an API key impossible to paste.
static void installEditMenu(void) {
	if (NSApp.mainMenu != nil) {
		return;
	}
	NSMenu *edit = [[NSMenu alloc] initWithTitle:@"Edit"];
	[edit addItemWithTitle:@"Undo" action:@selector(undo:) keyEquivalent:@"z"];
	NSMenuItem *redo = [edit addItemWithTitle:@"Redo" action:@selector(redo:) keyEquivalent:@"z"];
	redo.keyEquivalentModifierMask = NSEventModifierFlagCommand | NSEventModifierFlagShift;
	[edit addItem:[NSMenuItem separatorItem]];
	[edit addItemWithTitle:@"Cut" action:@selector(cut:) keyEquivalent:@"x"];
	[edit addItemWithTitle:@"Copy" action:@selector(copy:) keyEquivalent:@"c"];
	[edit addItemWithTitle:@"Paste" action:@selector(paste:) keyEquivalent:@"v"];
	[edit addItemWithTitle:@"Select All" action:@selector(selectAll:) keyEquivalent:@"a"];
	[edit addItem:[NSMenuItem separatorItem]];
	[edit addItemWithTitle:@"Close" action:@selector(performClose:) keyEquivalent:@"w"];

	NSMenuItem *editItem = [[NSMenuItem alloc] initWithTitle:@"Edit" action:nil keyEquivalent:@""];
	editItem.submenu = edit;
	NSMenu *menu = [[NSMenu alloc] initWithTitle:@""];
	[menu addItem:editItem];
	NSApp.mainMenu = menu;
}

void openSettingsWindow(const char *html) {
	NSString *page = [NSString stringWithUTF8String:html];
	dispatch_async(dispatch_get_main_queue(), ^{
		installEditMenu();
		if (settingsWindow == nil) {
			settingsWindow = [[VoxSteerSettingsWindow alloc] init];
		}
		if (!settingsWindow.window.isVisible) {
			// Reload so the form shows what is saved now, not what was last typed.
			[settingsWindow.webView loadHTMLString:page baseURL:nil];
		}
		if (@available(macOS 14.0, *)) {
			[NSApp activate];
		} else {
			[NSApp activateIgnoringOtherApps:YES];
		}
		[settingsWindow.window makeKeyAndOrderFront:nil];
	});
}

void closeSettingsWindow(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		[settingsWindow.window close];
	});
}
