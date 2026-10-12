//go:build darwin && masprobe
#import <Cocoa/Cocoa.h>
#import <sys/mount.h>
#import <stdatomic.h>
#import "bridge_darwin.h"
static NSURL *activeURL;
// Main-queue-only picker reference; it is valid while runModal is active.
static NSOpenPanel *activePanel;
static atomic_bool closing;
void ndg_cancel_picker(void) {
 atomic_store(&closing,true);
 dispatch_async(dispatch_get_main_queue(), ^{ if (activePanel) [activePanel cancel:nil]; });
}
static char *encode(NSDictionary *d) {
    NSData *data = [NSJSONSerialization dataWithJSONObject:d options:0 error:nil];
    return strdup([[[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding] autorelease].UTF8String);
}
static char *failure(NSString *kind) { return encode(@{@"error":kind}); }
void ndg_stop(void) {
    if (activeURL) { [activeURL stopAccessingSecurityScopedResource]; [activeURL release]; activeURL=nil; }
}
static char *activate(NSURL *url, BOOL stale) {
    if (!url || ![url startAccessingSecurityScopedResource]) return failure(@"authorization_required");
    ndg_stop(); activeURL=[url retain];
    BOOL dir=NO;
    if (![NSFileManager.defaultManager fileExistsAtPath:url.path isDirectory:&dir] || !dir) {
        ndg_stop(); return failure(@"source_unavailable");
    }
    // Refuse a selected symlink; NDG traversal also refuses descendant links/mounts.
    NSNumber *link=nil; [url getResourceValue:&link forKey:NSURLIsSymbolicLinkKey error:nil];
    if (link.boolValue) { ndg_stop(); return failure(@"unsafe_source"); }
    NSError *error=nil;
    NSData *bookmark=[url bookmarkDataWithOptions:(NSURLBookmarkCreationWithSecurityScope|NSURLBookmarkCreationSecurityScopeAllowOnlyReadAccess) includingResourceValuesForKeys:nil relativeToURL:nil error:&error];
    if (!bookmark) { ndg_stop(); return failure(@"bookmark_create_failed"); }
    struct statfs fs; BOOL network=NO;
    if (statfs(url.path.fileSystemRepresentation,&fs)==0) network=!(fs.f_flags & MNT_LOCAL);
    return encode(@{@"path":url.path,@"bookmark":[bookmark base64EncodedStringWithOptions:0],@"stale":@(stale),@"network":@(network)});
}
char *ndg_pick(const char *initial) {
    if (atomic_load(&closing)) return failure(@"shutting_down");
    __block char *result=NULL;
    void (^pick)(void)=^{
        if (atomic_load(&closing)) { result=failure(@"shutting_down"); return; }
        NSOpenPanel *p=NSOpenPanel.openPanel;
        p.canChooseFiles=NO; p.canChooseDirectories=YES; p.allowsMultipleSelection=NO;
        p.title=@"选择人工创建的 Phase 0 测试目录（只读）";
        if (initial && initial[0]) p.directoryURL=[NSURL fileURLWithPath:[NSString stringWithUTF8String:initial]];
        activePanel=p;
        NSModalResponse response=[p runModal];
        activePanel=nil;
        result=(response==NSModalResponseOK) ? activate(p.URL,NO) : failure(@"cancelled");
    };
    if (NSThread.isMainThread) pick(); else dispatch_sync(dispatch_get_main_queue(),pick);
    return result;
}
char *ndg_resolve(const char *raw) {
    @autoreleasepool {
        NSData *data=[[NSData alloc] initWithBase64EncodedString:[NSString stringWithUTF8String:raw] options:0];
        if (!data || data.length==0) { [data release]; return failure(@"authorization_required"); }
        BOOL stale=NO; NSError *error=nil;
        NSURL *url=[NSURL URLByResolvingBookmarkData:data options:(NSURLBookmarkResolutionWithSecurityScope|NSURLBookmarkResolutionWithoutUI|NSURLBookmarkResolutionWithoutMounting) relativeToURL:nil bookmarkDataIsStale:&stale error:&error];
        [data release];
        return activate(url,stale);
    }
}
char *ndg_support(void) {
    @autoreleasepool {
        NSURL *u=[NSFileManager.defaultManager URLsForDirectory:NSApplicationSupportDirectory inDomains:NSUserDomainMask].firstObject;
        return strdup([[u URLByAppendingPathComponent:@"NDG-MAS-Phase0"] path].UTF8String);
    }
}
