// Standalone research probe. Never linked into NDG or a signed release.
// No user source argument: only newly created disposable directories are read.
#import <Foundation/Foundation.h>
#include <stdlib.h>
#include <stdio.h>

static NSDictionary *readVolume(NSURL *url) {
    NSError *error = nil;
    NSDictionary *values = [url resourceValuesForKeys:@[
        NSURLVolumeIsLocalKey, NSURLVolumeIdentifierKey, NSURLVolumeUUIDStringKey,
        NSURLVolumeURLKey, NSURLVolumeURLForRemountingKey
    ] error:&error];
    return error ? nil : values;
}

int main(int argc, const char **argv) {
    @autoreleasepool {
        if (argc != 1) { puts("{\"status\":\"NO_SOURCE_ARGUMENTS_ALLOWED\"}"); return 2; }
        NSString *template = [NSTemporaryDirectory() stringByAppendingPathComponent:@"ndg-volume-research-XXXXXX"];
        char *buffer = strdup(template.fileSystemRepresentation);
        char *created = mkdtemp(buffer);
        if (!created) { free(buffer); puts("{\"status\":\"FIXTURE_FAILED\"}"); return 2; }
        NSURL *root = [NSURL fileURLWithPath:[NSString stringWithUTF8String:created] isDirectory:YES];
        free(buffer);
        NSFileManager *fm = NSFileManager.defaultManager;
        NSURL *a = [root URLByAppendingPathComponent:@"synthetic-a" isDirectory:YES];
        NSURL *b = [root URLByAppendingPathComponent:@"synthetic-b" isDirectory:YES];
        NSURL *moved = [root URLByAppendingPathComponent:@"synthetic-moved" isDirectory:YES];
        NSError *error = nil;
        BOOL prepared = [fm createDirectoryAtURL:a withIntermediateDirectories:NO attributes:nil error:&error]
            && [fm createDirectoryAtURL:b withIntermediateDirectories:NO attributes:nil error:&error];
        NSDictionary *before = prepared ? readVolume(a) : nil;
        NSDictionary *other = prepared ? readVolume(b) : nil;
        BOOL renamed = prepared && [fm moveItemAtURL:a toURL:moved error:&error];
        NSDictionary *after = renamed ? readVolume(moved) : nil;
        id identity = before[NSURLVolumeIdentifierKey];
        BOOL sameDirectories = identity && [identity isEqual:other[NSURLVolumeIdentifierKey]];
        BOOL sameAfterMove = identity && [identity isEqual:after[NSURLVolumeIdentifierKey]];
        BOOL cleanup = [fm removeItemAtURL:root error:&error];
        BOOL readOK = before && other && after;
        NSDictionary *result = @{
            @"status": (readOK && cleanup) ? @"LOCAL_RESEARCH_COMPLETE" : @"INCONCLUSIVE",
            @"metadata_read": @(readOK), @"local_value_present": @(before[NSURLVolumeIsLocalKey] != nil),
            @"is_local": @([before[NSURLVolumeIsLocalKey] boolValue]),
            @"volume_identifier_present": @(identity != nil),
            @"volume_uuid_present": @(before[NSURLVolumeUUIDStringKey] != nil),
            @"volume_root_present": @(before[NSURLVolumeURLKey] != nil),
            @"remount_url_present": @(before[NSURLVolumeURLForRemountingKey] != nil),
            @"different_directories_same_volume_identifier": @(sameDirectories),
            @"rename_same_volume_identifier": @(sameAfterMove), @"fixture_cleanup": @(cleanup)
        };
        NSData *json = [NSJSONSerialization dataWithJSONObject:result options:NSJSONWritingSortedKeys error:nil];
        // Never print NSURLs, UUIDs, opaque identifiers, errors or host paths.
        if (!json) { puts("{\"status\":\"ENCODING_FAILED\"}"); return 2; }
        fwrite(json.bytes, 1, json.length, stdout); putchar('\n');
        return readOK && cleanup ? 0 : 2;
    }
}
