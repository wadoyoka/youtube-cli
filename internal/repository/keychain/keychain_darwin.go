//go:build darwin && cgo

package keychain

/*
#cgo CFLAGS: -Wno-deprecated-declarations
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#define __STDC_WANT_LIB_EXT1__ 1
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <Security/Security.h>
#include <CoreFoundation/CoreFoundation.h>

static CFStringRef mkstr(const char *s) {
	return CFStringCreateWithCString(kCFAllocatorDefault, s, kCFStringEncodingUTF8);
}

static CFMutableDictionaryRef query(const char *service, const char *account) {
	CFMutableDictionaryRef q = CFDictionaryCreateMutable(kCFAllocatorDefault, 0,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	CFStringRef s = mkstr(service), a = mkstr(account);
	CFDictionarySetValue(q, kSecClass, kSecClassGenericPassword);
	CFDictionarySetValue(q, kSecAttrService, s);
	CFDictionarySetValue(q, kSecAttrAccount, a);
	CFRelease(s); CFRelease(a);
	return q;
}

static OSStatus kc_add(const char *service, const char *account, const char *label,
		const char *comment, const void *data, long len) {
	SecTrustedApplicationRef me = NULL;
	OSStatus st = SecTrustedApplicationCreateFromPath(NULL, &me);
	if (st != errSecSuccess) return st;
	CFArrayRef trusted = CFArrayCreate(kCFAllocatorDefault, (const void **)&me, 1, &kCFTypeArrayCallBacks);
	CFRelease(me);
	CFStringRef lbl = mkstr(label);
	SecAccessRef access = NULL;
	st = SecAccessCreate(lbl, trusted, &access);
	CFRelease(trusted);
	if (st != errSecSuccess) { CFRelease(lbl); return st; }
	CFMutableDictionaryRef q = query(service, account);
	CFStringRef cmt = mkstr(comment);
	CFDataRef d = CFDataCreate(kCFAllocatorDefault, data, len);
	CFDictionarySetValue(q, kSecAttrLabel, lbl);
	CFDictionarySetValue(q, kSecAttrComment, cmt);
	CFDictionarySetValue(q, kSecAttrAccess, access);
	CFDictionarySetValue(q, kSecValueData, d);
	st = SecItemAdd(q, NULL);
	CFRelease(d); CFRelease(cmt); CFRelease(lbl); CFRelease(access); CFRelease(q);
	return st;
}

// *out は呼び出し側が kc_free_secret で解放する。
static OSStatus kc_get(const char *service, const char *account, void **out, long *outLen) {
	CFMutableDictionaryRef q = query(service, account);
	CFDictionarySetValue(q, kSecReturnData, kCFBooleanTrue);
	CFDictionarySetValue(q, kSecMatchLimit, kSecMatchLimitOne);
	CFTypeRef res = NULL;
	OSStatus st = SecItemCopyMatching(q, &res);
	CFRelease(q);
	if (st != errSecSuccess) return st;
	long n = (long)CFDataGetLength((CFDataRef)res);
	void *buf = malloc(n > 0 ? n : 1);
	memcpy(buf, CFDataGetBytePtr((CFDataRef)res), n);
	*out = buf; *outLen = n;
	CFRelease(res);
	return st;
}

static OSStatus kc_comment(const char *service, const char *account, char *comment, int commentLen) {
	CFMutableDictionaryRef q = query(service, account);
	CFDictionarySetValue(q, kSecReturnAttributes, kCFBooleanTrue);
	CFDictionarySetValue(q, kSecMatchLimit, kSecMatchLimitOne);
	CFTypeRef res = NULL;
	OSStatus st = SecItemCopyMatching(q, &res);
	CFRelease(q);
	if (st != errSecSuccess) return st;
	comment[0] = 0;
	CFTypeRef c = CFDictionaryGetValue((CFDictionaryRef)res, kSecAttrComment);
	if (c != NULL && CFGetTypeID(c) == CFStringGetTypeID()) {
		CFStringGetCString((CFStringRef)c, comment, commentLen, kCFStringEncodingUTF8);
	}
	CFRelease(res);
	return st;
}

static OSStatus kc_delete(const char *service, const char *account) {
	CFMutableDictionaryRef q = query(service, account);
	OSStatus st = SecItemDelete(q);
	CFRelease(q);
	return st;
}

static OSStatus kc_self_sig(uint32_t *staticFlags, uint32_t *dynStatus) {
	SecCodeRef code = NULL;
	OSStatus st = SecCodeCopySelf(kSecCSDefaultFlags, &code);
	if (st != errSecSuccess) return st;
	CFDictionaryRef info = NULL;
	st = SecCodeCopySigningInformation((SecStaticCodeRef)code,
		kSecCSSigningInformation | kSecCSDynamicInformation, &info);
	CFRelease(code);
	if (st != errSecSuccess) return st;
	*staticFlags = 0; *dynStatus = 0;
	CFNumberRef f = CFDictionaryGetValue(info, kSecCodeInfoFlags);
	if (f != NULL) CFNumberGetValue(f, kCFNumberSInt32Type, staticFlags);
	CFNumberRef s = CFDictionaryGetValue(info, kSecCodeInfoStatus);
	if (s != NULL) CFNumberGetValue(s, kCFNumberSInt32Type, dynStatus);
	CFRelease(info);
	return errSecSuccess;
}

// コンパイラに消されないよう memset_s で 0 にしてから free する。
static void kc_free_secret(void *p, long n) {
	if (p == NULL) return;
	memset_s(p, (rsize_t)(n > 0 ? n : 1), 0, (rsize_t)(n > 0 ? n : 1));
	free(p);
}
*/
import "C"

import "unsafe"

// commentBufSize は Comment が受け取る目印の最大バイト数。
const commentBufSize = 1024

// Add は、ACL で信頼するアプリをこの実行ファイルだけにして、汎用パスワードの項目を作る。
func Add(service, account, label, comment string, secret []byte) Status {
	cs, ca, cl, cc := C.CString(service), C.CString(account), C.CString(label), C.CString(comment)
	defer C.free(unsafe.Pointer(cs))
	defer C.free(unsafe.Pointer(ca))
	defer C.free(unsafe.Pointer(cl))
	defer C.free(unsafe.Pointer(cc))
	data := C.CBytes(secret)
	defer C.kc_free_secret(data, C.long(len(secret)))
	return Status(C.kc_add(cs, ca, cl, cc, data, C.long(len(secret))))
}

// Get は値を読む。ACL の確認（ダイアログ）が起きうる。
func Get(service, account string) ([]byte, Status) {
	cs, ca := C.CString(service), C.CString(account)
	defer C.free(unsafe.Pointer(cs))
	defer C.free(unsafe.Pointer(ca))
	var out unsafe.Pointer
	var n C.long
	st := Status(C.kc_get(cs, ca, &out, &n)) //nolint:gocritic // cgo が生成する引数検査のコードを dupSubExpr が誤検知する
	if st != StatusOK {
		return nil, st
	}
	defer C.kc_free_secret(out, n)
	return C.GoBytes(out, C.int(n)), st
}

// Comment は、値を読まずに属性だけを問い合わせ、kSecAttrComment を返す（なければ ""）。ダイアログは起きない。
func Comment(service, account string) (string, Status) {
	cs, ca := C.CString(service), C.CString(account)
	defer C.free(unsafe.Pointer(cs))
	defer C.free(unsafe.Pointer(ca))
	buf := (*C.char)(C.malloc(commentBufSize))
	defer C.free(unsafe.Pointer(buf))
	st := Status(C.kc_comment(cs, ca, buf, commentBufSize))
	if st != StatusOK {
		return "", st
	}
	return C.GoString(buf), st
}

// Delete は項目を消す。
func Delete(service, account string) Status {
	cs, ca := C.CString(service), C.CString(account)
	defer C.free(unsafe.Pointer(cs))
	defer C.free(unsafe.Pointer(ca))
	return Status(C.kc_delete(cs, ca))
}

// SelfSignature は、この実行ファイル自身の署名の印（kSecCodeInfoFlags）と、
// 実行中の状態語（kSecCodeInfoStatus）を返す。
func SelfSignature() (staticFlags, dynamicStatus uint32, st Status) {
	var f, d C.uint32_t
	st = Status(C.kc_self_sig(&f, &d))
	return uint32(f), uint32(d), st
}
