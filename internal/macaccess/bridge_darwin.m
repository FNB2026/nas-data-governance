//go:build darwin && mas
#import <Cocoa/Cocoa.h>
#import <sys/mount.h>
#import <sys/stat.h>
#import <stdatomic.h>
#import "bridge_darwin.h"
static atomic_bool closing;
static NSOpenPanel *activePanel; // main queue only
static char *encode(NSDictionary *d) {
 NSData *data=[NSJSONSerialization dataWithJSONObject:d options:0 error:nil];
 if (!data) return strdup("{\"Error\":\"AUTHORIZATION_REQUIRED\"}");
 return strdup([[[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding] autorelease].UTF8String);
}
static char *failure(NSString *kind) {return encode(@{@"Error":kind});}
void ndg_access_release(void *scope) {
 if (scope) {NSURL *url=(NSURL *)scope; [url stopAccessingSecurityScopedResource]; [url release];}
}
void ndg_access_cancel_picker(void) {
 atomic_store(&closing,true);
 dispatch_async(dispatch_get_main_queue(), ^{if (activePanel) [activePanel cancel:nil];});
}
static char *activate(NSURL *url, void **scope) {
 if (!url || atomic_load(&closing)) return failure(@"AUTHORIZATION_REQUIRED");
 if (![url startAccessingSecurityScopedResource]) return failure(@"AUTHORIZATION_REQUIRED");
 struct stat st; struct statfs fs;
 if (lstat(url.path.fileSystemRepresentation,&st)!=0) { int savedErrno=errno; [url stopAccessingSecurityScopedResource]; return failure(savedErrno==ENOENT ? @"SOURCE_UNAVAILABLE":@"AUTHORIZATION_REQUIRED"); }
 if (!S_ISDIR(st.st_mode) || statfs(url.path.fileSystemRepresentation,&fs)!=0) { [url stopAccessingSecurityScopedResource];return failure(@"AUTHORIZATION_REQUIRED"); }
 NSError *err=nil;
 NSData *data=[url bookmarkDataWithOptions:(NSURLBookmarkCreationWithSecurityScope|NSURLBookmarkCreationSecurityScopeAllowOnlyReadAccess) includingResourceValuesForKeys:nil relativeToURL:nil error:&err];
 if (!data) {[url stopAccessingSecurityScopedResource];return failure(@"AUTHORIZATION_REQUIRED");}
 BOOL network=!(fs.f_flags&MNT_LOCAL);
 // Bind grant to mount source and root inode, not just a path. Network mounts
 // may change local device number after remount; their mount source must match.
 NSString *identity=network ? [NSString stringWithFormat:@"%s:%llu",fs.f_mntfromname,(unsigned long long)st.st_ino] : [NSString stringWithFormat:@"%llu:%llu",(unsigned long long)st.st_dev,(unsigned long long)st.st_ino];
 *scope=(void *)[url retain];
 return encode(@{@"Root":url.path,@"Bookmark":[data base64EncodedStringWithOptions:0],@"Identity":identity,@"Network":@(network)});
}
char *ndg_access_pick(void **scope) {
 *scope=NULL;
 if (atomic_load(&closing)) return failure(@"AUTHORIZATION_REQUIRED");
 __block char *result=NULL;
 void(^pick)(void)=^{
  @autoreleasepool {
   if (atomic_load(&closing)) {result=failure(@"AUTHORIZATION_REQUIRED");return;}
   NSOpenPanel *panel=NSOpenPanel.openPanel;
   panel.canChooseFiles=NO;panel.canChooseDirectories=YES;panel.allowsMultipleSelection=NO;
   panel.title=@"授权只读扫描目录";activePanel=panel;
   NSModalResponse response=[panel runModal];activePanel=nil;
   result=response==NSModalResponseOK?activate(panel.URL,scope):failure(@"AUTHORIZATION_REQUIRED");
  }
 };
 if (NSThread.isMainThread) pick();else dispatch_sync(dispatch_get_main_queue(),pick);
 return result;
}
char *ndg_access_resolve(const char *raw,const char *expectedRoot,int expectedNetwork,void **scope) {
 *scope=NULL;
 @autoreleasepool {
  NSData *data=[[NSData alloc] initWithBase64EncodedString:[NSString stringWithUTF8String:raw] options:0];
  if (!data||data.length==0){[data release];return failure(@"AUTHORIZATION_REQUIRED");}
  // A valid bookmark may refuse resolution when WithoutMounting is set and
  // its volume is absent. Distinguish that from malformed/denied authority
  // using bookmark metadata and the trusted saved root's actual mount state.
  if (expectedNetwork && expectedRoot && [NSURL resourceValuesForKeys:@[NSURLNameKey] fromBookmarkData:data]!=nil) {
   struct statfs fs;
   int result=statfs(expectedRoot,&fs);
   if ((result==0 && (fs.f_flags&MNT_LOCAL)) || (result!=0 && errno==ENOENT)) { [data release];return failure(@"SOURCE_UNAVAILABLE"); }
  }
  NSError *error=nil;BOOL stale=NO;
  NSURL *url=[NSURL URLByResolvingBookmarkData:data options:(NSURLBookmarkResolutionWithSecurityScope|NSURLBookmarkResolutionWithoutUI|NSURLBookmarkResolutionWithoutMounting) relativeToURL:nil bookmarkDataIsStale:&stale error:&error];[data release];
  if (!url) {return failure([error.domain isEqualToString:NSCocoaErrorDomain] && error.code==NSFileReadNoSuchFileError ? @"SOURCE_UNAVAILABLE":@"AUTHORIZATION_REQUIRED");}
  if (expectedNetwork) {
   struct statfs fs;
   if (statfs(url.path.fileSystemRepresentation,&fs)==0 && (fs.f_flags&MNT_LOCAL)) return failure(@"SOURCE_UNAVAILABLE");
  }
  return activate(url,scope);
 }
}
char *ndg_access_support(void) {
 @autoreleasepool {NSURL *u=[NSFileManager.defaultManager URLsForDirectory:NSApplicationSupportDirectory inDomains:NSUserDomainMask].firstObject;return strdup([[u URLByAppendingPathComponent:@"NDG"] path].UTF8String);}
}
