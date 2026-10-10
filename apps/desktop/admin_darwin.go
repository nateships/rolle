//go:build darwin

package main

/*
#cgo LDFLAGS: -framework Security
#include <Security/Security.h>
#include <stdlib.h>
#include <string.h>

// rolleRunAsAdmin asks for the administrator right with prompt, then runs
// /bin/sh -c script as root. It reads the output of the script until the
// script closes it and puts it in a malloc buffer in out. The caller frees
// out. The result is an Authorization Services status.
static OSStatus rolleRunAsAdmin(const char *prompt, const char *script, char **out, size_t *outLen) {
	*out = NULL;
	*outLen = 0;
	AuthorizationRef ref;
	OSStatus st = AuthorizationCreate(NULL, kAuthorizationEmptyEnvironment, kAuthorizationFlagDefaults, &ref);
	if (st != errAuthorizationSuccess) return st;
	AuthorizationItem right = { kAuthorizationRightExecute, 0, NULL, 0 };
	AuthorizationRights rights = { 1, &right };
	AuthorizationItem promptItem = { kAuthorizationEnvironmentPrompt, strlen(prompt), (void *)prompt, 0 };
	AuthorizationEnvironment env = { 1, &promptItem };
	AuthorizationFlags flags = kAuthorizationFlagInteractionAllowed | kAuthorizationFlagExtendRights | kAuthorizationFlagPreAuthorize;
	st = AuthorizationCopyRights(ref, &rights, &env, flags, NULL);
	if (st == errAuthorizationSuccess) {
		char *args[] = { "-c", (char *)script, NULL };
		FILE *pipe = NULL;
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
		st = AuthorizationExecuteWithPrivileges(ref, "/bin/sh", kAuthorizationFlagDefaults, args, &pipe);
#pragma clang diagnostic pop
		if (st == errAuthorizationSuccess && pipe != NULL) {
			size_t cap = 4096, n;
			char *buf = malloc(cap);
			while (buf != NULL && (n = fread(buf + *outLen, 1, cap - *outLen, pipe)) > 0) {
				*outLen += n;
				if (*outLen == cap) {
					char *grown = realloc(buf, cap * 2);
					if (grown == NULL) break;
					buf = grown;
					cap *= 2;
				}
			}
			*out = buf;
			fclose(pipe);
		}
	}
	AuthorizationFree(ref, kAuthorizationFlagDestroyRights);
	return st;
}
*/
import "C"

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"
)

// runAsAdmin runs script as root after the macOS administrator prompt, which
// shows prompt. The prompt comes from Authorization Services in this
// process, not from osascript. macOS then offers Touch ID to an
// administrator when the app is notarized. A dev build gets the password
// field only.
func runAsAdmin(prompt, script string) error {
	cPrompt, cScript := C.CString(prompt), C.CString(adminScript(script))
	defer C.free(unsafe.Pointer(cPrompt))
	defer C.free(unsafe.Pointer(cScript))
	var out *C.char
	var n C.size_t
	st := C.rolleRunAsAdmin(cPrompt, cScript, &out, &n)
	defer C.free(unsafe.Pointer(out))
	switch st {
	case C.errAuthorizationSuccess:
	case C.errAuthorizationCanceled:
		return installError(nil, nil, true)
	default:
		return fmt.Errorf("install failed: administrator authorization failed (%d)", int(st))
	}
	output, code, pid, ok := adminResult(C.GoBytes(unsafe.Pointer(out), C.int(n)))
	if !ok {
		return installError(output, errors.New("the administrator command did not finish"), false)
	}
	// The shell is a child of this process. Reap it.
	_, _ = syscall.Wait4(pid, nil, 0, nil)
	if code != 0 {
		return installError(output, fmt.Errorf("exit status %d", code), false)
	}
	return nil
}
