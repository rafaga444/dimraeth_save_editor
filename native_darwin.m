//go:build darwin

#import <Cocoa/Cocoa.h>
#import "native_darwin.h"
extern void onNativeEvent(int ident);

@interface WorkshopView : NSView
@end
@implementation WorkshopView
- (BOOL)isFlipped { return YES; }
@end

@interface WorkshopController : NSObject <NSWindowDelegate,NSTableViewDataSource,NSTableViewDelegate,NSTextFieldDelegate>
@end
static NSWindow *window;
static WorkshopView *root;
static NSView *pages[2];
static NSMutableDictionary *controls;
static NSMutableDictionary *tableRows;
static WorkshopController *controller;
static BOOL changing;

@implementation WorkshopController
- (void)control:(id)sender { onNativeEvent((int)[sender tag]); }
- (void)quit:(id)sender { onNativeEvent(1); }
- (void)controlTextDidChange:(NSNotification *)notification { if (!changing) onNativeEvent((int)[notification.object tag]); }
- (BOOL)windowShouldClose:(id)sender { onNativeEvent(1); return NO; }
- (NSInteger)numberOfRowsInTableView:(NSTableView *)table { return [tableRows[@(table.tag)] count]; }
- (id)tableView:(NSTableView *)table objectValueForTableColumn:(NSTableColumn *)column row:(NSInteger)row { return tableRows[@(table.tag)][row]; }
- (void)tableViewSelectionDidChange:(NSNotification *)notification { if (!changing) onNativeEvent((int)[notification.object tag]); }
@end

static NSString *S(const char *s) { return [NSString stringWithUTF8String:s ? s : ""] ?: @""; }
void nativeInit(void) {
 @autoreleasepool {
  [NSApplication sharedApplication];
  [NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
  controller=[WorkshopController new]; controls=[NSMutableDictionary new]; tableRows=[NSMutableDictionary new];
  NSMenu *menu=[NSMenu new];NSMenuItem *appItem=[NSMenuItem new];[menu addItem:appItem];
  NSMenu *appMenu=[NSMenu new];NSMenuItem *quit=[[NSMenuItem alloc] initWithTitle:@"Quit Dimraeth Editor" action:@selector(quit:) keyEquivalent:@"q"];quit.target=controller;[appMenu addItem:quit];appItem.submenu=appMenu;
  NSMenuItem *editItem=[[NSMenuItem alloc] initWithTitle:@"Edit" action:nil keyEquivalent:@""];NSMenu *edit=[[NSMenu alloc] initWithTitle:@"Edit"];
  [edit addItemWithTitle:@"Undo" action:@selector(undo:) keyEquivalent:@"z"];
  [edit addItemWithTitle:@"Cut" action:@selector(cut:) keyEquivalent:@"x"];
  [edit addItemWithTitle:@"Copy" action:@selector(copy:) keyEquivalent:@"c"];
  [edit addItemWithTitle:@"Paste" action:@selector(paste:) keyEquivalent:@"v"];
  [edit addItemWithTitle:@"Select All" action:@selector(selectAll:) keyEquivalent:@"a"];
  editItem.submenu=edit;[menu addItem:editItem];NSApp.mainMenu=menu;
  NSRect screen=[NSScreen mainScreen].visibleFrame;CGFloat scale=MIN(1.0,MIN((screen.size.height-45)/895.0,(screen.size.width-40)/1130.0));
  window=[[NSWindow alloc] initWithContentRect:NSMakeRect(0,0,1130*scale,895*scale) styleMask:NSWindowStyleMaskTitled|NSWindowStyleMaskClosable|NSWindowStyleMaskMiniaturizable backing:NSBackingStoreBuffered defer:NO];
  window.title=@"Dimraeth Editor";window.delegate=controller;
  root=[[WorkshopView alloc] initWithFrame:NSMakeRect(0,0,1130*scale,895*scale)];root.bounds=NSMakeRect(0,0,1130,895);window.contentView=root;
  NSTabView *tabs=[[NSTabView alloc] initWithFrame:NSMakeRect(15,112,1100,721)];
  NSArray *titles=@[@"Save editor",@"Patcher"];
  for(int i=0;i<2;i++){NSTabViewItem *item=[[NSTabViewItem alloc] initWithIdentifier:@(i)];item.label=titles[i];pages[i]=[[WorkshopView alloc] initWithFrame:NSMakeRect(0,0,1090,686)];item.view=pages[i];[tabs addTabViewItem:item];}
  [root addSubview:tabs];[window center];
 }
}
void nativeAdd(char *kind,int ident,int page,int x,int y,int w,int h,char *text) {
 @autoreleasepool {
  NSRect rect=NSMakeRect(x,y,w,h);NSString *k=S(kind);NSView *view;id control;
  if([k isEqualToString:@"button"]){NSButton *b=[[NSButton alloc] initWithFrame:rect];b.title=S(text);b.bezelStyle=NSBezelStyleRounded;b.target=controller;b.action=@selector(control:);b.tag=ident;if(ident==24){b.keyEquivalent=@"s";}control=view=b;}
  else if([k isEqualToString:@"toggle"]){NSButton *b=[[NSButton alloc] initWithFrame:rect];[b setButtonType:NSButtonTypeSwitch];b.title=S(text);b.target=controller;b.action=@selector(control:);b.tag=ident;control=view=b;}
  else if([k isEqualToString:@"combo"]){NSPopUpButton *p=[[NSPopUpButton alloc] initWithFrame:rect pullsDown:NO];p.target=controller;p.action=@selector(control:);p.tag=ident;control=view=p;}
  else if([k isEqualToString:@"list"]){
   NSScrollView *scroll=[[NSScrollView alloc] initWithFrame:rect];scroll.hasVerticalScroller=YES;scroll.hasHorizontalScroller=YES;scroll.borderType=NSBezelBorder;
   NSTableView *t=[[NSTableView alloc] initWithFrame:NSMakeRect(0,0,w,h)];t.tag=ident;t.headerView=nil;t.rowHeight=23;t.usesAlternatingRowBackgroundColors=YES;t.allowsEmptySelection=YES;t.allowsMultipleSelection=NO;t.delegate=controller;t.dataSource=controller;
   NSTableColumn *column=[[NSTableColumn alloc] initWithIdentifier:@"value"];column.width=w-22;column.editable=NO;[column.dataCell setFont:[NSFont systemFontOfSize:12]];[t addTableColumn:column];t.columnAutoresizingStyle=NSTableViewNoColumnAutoresizing;scroll.documentView=t;tableRows[@(ident)]=@[];control=t;view=scroll;
  }else{
   NSTextField *field=[[NSTextField alloc] initWithFrame:rect];field.tag=ident;field.stringValue=S(text);field.font=[NSFont systemFontOfSize:13];
   if([k isEqualToString:@"entry"]){field.delegate=controller;field.cell.scrollable=YES;}
   else{field.editable=NO;field.selectable=YES;field.bordered=NO;field.drawsBackground=NO;field.cell.wraps=YES;field.cell.scrollable=NO;field.lineBreakMode=NSLineBreakByWordWrapping;if([k isEqualToString:@"heading"]){field.font=[NSFont boldSystemFontOfSize:16];}else{field.textColor=[NSColor secondaryLabelColor];}}
   control=view=field;
  }
  controls[@(ident)]=control;[control setAccessibilityIdentifier:[NSString stringWithFormat:@"control-%d",ident]];
  NSDictionary *names=@{@10:@"Game folder",@20:@"Save file path",@30:@"Parameter section",@31:@"Search parameters",@32:@"Parameters",@34:@"Parameter value",@35:@"Boolean value",@40:@"Search items",@41:@"Item",@42:@"Quantity",@44:@"Inventory",@50:@"Drop chance multiplier",@51:@"Rarity",@54:@"RNG eliminator",@55:@"Forced stars",@59:@"Max level cap"};
  [control setAccessibilityLabel:names[@(ident)] ?: S(text)];
  [(page<0?root:pages[page]) addSubview:view];
 }
}
char *nativeText(int ident){id c=controls[@(ident)];return strdup([[c stringValue] UTF8String] ?: "");}
void nativeSetText(int ident,char *text){changing=YES;id c=controls[@(ident)];[c setStringValue:S(text)];if([c isKindOfClass:[NSTextField class]]&&![c isEditable]){[c setToolTip:S(text)];}[c displayIfNeeded];changing=NO;}
void nativeOptions(int ident,char *json){
 changing=YES;@autoreleasepool {NSArray *items=[NSJSONSerialization JSONObjectWithData:[S(json) dataUsingEncoding:NSUTF8StringEncoding] options:0 error:nil];id c=controls[@(ident)];
 if([c isKindOfClass:[NSTableView class]]){tableRows[@(ident)]=items;NSTableView *t=c;CGFloat width=t.enclosingScrollView.contentSize.width;for(NSString *s in items){width=MAX(width,[s sizeWithAttributes:@{NSFontAttributeName:[NSFont systemFontOfSize:12]}].width+20);}[[t.tableColumns firstObject] setWidth:width];[t reloadData];}
 else{[c removeAllItems];[c addItemsWithTitles:items];}}
 changing=NO;
}
int nativeSelection(int ident){id c=controls[@(ident)];if([c isKindOfClass:[NSButton class]]&&![c isKindOfClass:[NSPopUpButton class]])return [c state]==NSControlStateValueOn;if([c isKindOfClass:[NSTableView class]])return (int)[c selectedRow];return (int)[c indexOfSelectedItem];}
void nativeSelect(int ident,int index){changing=YES;id c=controls[@(ident)];if([c isKindOfClass:[NSButton class]]&&![c isKindOfClass:[NSPopUpButton class]]){[c setState:index==1?NSControlStateValueOn:NSControlStateValueOff];changing=NO;return;}if([c isKindOfClass:[NSTableView class]]){if(index>=0&&index<[c numberOfRows]){[c selectRowIndexes:[NSIndexSet indexSetWithIndex:index] byExtendingSelection:NO];[c scrollRowToVisible:index];}else{[c deselectAll:nil];}}else{if(index>=0&&index<[c numberOfItems])[c selectItemAtIndex:index];else[c selectItem:nil];}changing=NO;}
void nativeEnable(int ident,int enabled){[controls[@(ident)] setEnabled:enabled!=0];}
void nativeShow(int ident,int visible){[controls[@(ident)] setHidden:visible==0];}
char *nativePick(int folder){@autoreleasepool{NSOpenPanel *p=[NSOpenPanel openPanel];p.title=folder?@"Select game folder":@"Open Dimraeth save";p.prompt=folder?@"Select":@"Open";p.canChooseFiles=!folder;p.canChooseDirectories=folder;p.allowsMultipleSelection=NO;if([p runModal]==NSModalResponseOK)return strdup(p.URL.path.UTF8String);return strdup("");}}
void nativeAlert(char *title,char *message){@autoreleasepool{NSAlert *a=[NSAlert new];a.messageText=S(title);a.informativeText=S(message);[a addButtonWithTitle:@"OK"];[a runModal];}}
int nativeConfirm(char *title,char *message){@autoreleasepool{NSAlert *a=[NSAlert new];a.messageText=S(title);a.informativeText=S(message);[a addButtonWithTitle:@"Discard changes"];[a addButtonWithTitle:@"Cancel"];return [a runModal]==NSAlertFirstButtonReturn;}}
void nativeRun(void){@autoreleasepool{[window makeKeyAndOrderFront:nil];[NSApp activateIgnoringOtherApps:YES];[NSApp run];}}
void nativeStop(void){[window orderOut:nil];[NSApp stop:nil];}
