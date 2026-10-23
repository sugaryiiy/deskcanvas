//go:build darwin

#import <Cocoa/Cocoa.h>
#import <Carbon/Carbon.h>

// Implemented on the Go side (platform_darwin.go); called by the Carbon
// hotkey handler below whenever ⌥⌘D is pressed anywhere in the system.
extern void dcHotkeyPressed(void);

// The Wails window is the app's main window once the webview exists.
// Before that, fall back to the largest window the app owns.
static NSWindow *dc_main_window(void) {
    NSWindow *w = [NSApp mainWindow];
    if (w != nil) {
        return w;
    }
    NSWindow *best = nil;
    CGFloat bestArea = 0;
    for (NSWindow *cand in [NSApp windows]) {
        CGFloat area = cand.frame.size.width * cand.frame.size.height;
        if (area > bestArea) {
            bestArea = area;
            best = cand;
        }
    }
    return best;
}

// Core M0 experiment. Everything the product promises lives or dies in
// this one function:
//
//   locked: level = CGWindowLevelForKey(desktopIcon) + offset, mouse
//           events ignored. The canvas should read as part of the
//           wallpaper: covered by every normal window, never blocking
//           desktop-icon clicks. Whether cards should sit just below
//           the icons (offset -1, the default), level with them (0) or
//           just above (+1) is exactly what the validation run decides
//           — that is why the offset is a parameter, not a constant.
//   edit:   normal window level, mouse events on, window made key so
//           the user can actually type and drag.
static void dc_apply(int locked, int levelOffset, int attempts) {
    NSWindow *w = dc_main_window();
    if (w == nil) {
        // Window not created yet (early OnStartup). Retry briefly on
        // the main queue instead of silently dropping the request.
        if (attempts < 20) {
            dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(250 * NSEC_PER_MSEC)),
                           dispatch_get_main_queue(), ^{
                               dc_apply(locked, levelOffset, attempts + 1);
                           });
        } else {
            NSLog(@"deskcanvas: no window found after retries; apply dropped");
        }
        return;
    }

    NSScreen *screen = [w screen] ?: [NSScreen mainScreen];
    if (screen != nil) {
        [w setFrame:[screen frame] display:YES];
    }
    [w setStyleMask:NSWindowStyleMaskBorderless];
    [w setOpaque:NO];
    [w setBackgroundColor:[NSColor clearColor]];
    [w setHasShadow:NO];
    [w setHidesOnDeactivate:NO];
    // Live on every Space, never swept up by Mission Control cycling.
    [w setCollectionBehavior:NSWindowCollectionBehaviorCanJoinAllSpaces |
                             NSWindowCollectionBehaviorStationary |
                             NSWindowCollectionBehaviorIgnoresCycle];

    if (locked) {
        // Accessory: no Dock icon, never the active app while sunk.
        [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
        CGWindowLevel iconLevel = CGWindowLevelForKey(kCGDesktopIconWindowLevelKey);
        [w setLevel:(iconLevel + levelOffset)];
        [w setIgnoresMouseEvents:YES];
        [w orderFrontRegardless];
    } else {
        // Regular while editing so the window can become key and take
        // keyboard input like a normal app; sinks back to Accessory
        // when locked again.
        [NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
        [w setLevel:NSNormalWindowLevel];
        [w setIgnoresMouseEvents:NO];
        // The user explicitly summoned edit mode with the hotkey, so
        // activating the app here is intended, not focus-stealing.
        [NSApp activateIgnoringOtherApps:YES];
        [w makeKeyAndOrderFront:nil];
    }
    NSLog(@"deskcanvas: applied locked=%d level=%ld offset=%d",
          locked, (long)[w level], levelOffset);
}

void dc_apply_mode(int locked, int levelOffset) {
    dispatch_async(dispatch_get_main_queue(), ^{
        dc_apply(locked, levelOffset, 0);
    });
}

// --- Global hotkey ⌥⌘D (Carbon RegisterEventHotKey) ---------------------
// Carbon hotkeys need no Accessibility permission and no event tap,
// which keeps M0 free of permission prompts. kVK_ANSI_D = 0x02.

static EventHotKeyRef gHotKeyRef = NULL;

static OSStatus dc_hotkey_handler(EventHandlerCallRef nextHandler, EventRef event, void *userData) {
    dcHotkeyPressed();
    return noErr;
}

void dc_install_hotkey(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (gHotKeyRef != NULL) {
            return;
        }
        EventTypeSpec spec;
        spec.eventClass = kEventClassKeyboard;
        spec.eventKind = kEventHotKeyPressed;
        InstallApplicationEventHandler(NewEventHandlerUPP(dc_hotkey_handler), 1, &spec, NULL, NULL);
        EventHotKeyID hid;
        hid.signature = 'DCNV';
        hid.id = 1;
        OSStatus st = RegisterEventHotKey(kVK_ANSI_D, cmdKey | optionKey, hid,
                                          GetApplicationEventTarget(), 0, &gHotKeyRef);
        NSLog(@"deskcanvas: hotkey ⌥⌘D register status=%d", (int)st);
    });
}
