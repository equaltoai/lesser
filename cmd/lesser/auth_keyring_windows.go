//go:build windows
// +build windows

package main

import (
	"encoding/base64"
	"fmt"
	"os/exec"
	"strings"
)

func keyringIsAvailable() bool {
	_, err := exec.LookPath("powershell.exe")
	return err == nil
}

func keyringLoadSecret(account string) (string, error) {
	target := fmt.Sprintf("%s/%s", lesserCLIKeyringServiceName, account)

	script := fmt.Sprintf(`
Add-Type -AssemblyName System.Runtime.WindowsRuntime
$target = '%s'

Add-Type @"
using System;
using System.Runtime.InteropServices;
using System.Text;

public class CredentialManager {
    [DllImport("advapi32.dll", SetLastError = true, CharSet = CharSet.Unicode)]
    public static extern bool CredRead(string target, int type, int flags, out IntPtr credential);

    [DllImport("advapi32.dll", SetLastError = true)]
    public static extern bool CredFree(IntPtr credential);

    [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]
    public struct CREDENTIAL {
        public int Flags;
        public int Type;
        public string TargetName;
        public string Comment;
        public System.Runtime.InteropServices.ComTypes.FILETIME LastWritten;
        public int CredentialBlobSize;
        public IntPtr CredentialBlob;
        public int Persist;
        public int AttributeCount;
        public IntPtr Attributes;
        public string TargetAlias;
        public string UserName;
    }

    public static string GetPassword(string target) {
        IntPtr credPtr;
        if (CredRead(target, 1, 0, out credPtr)) {
            try {
                CREDENTIAL cred = (CREDENTIAL)Marshal.PtrToStructure(credPtr, typeof(CREDENTIAL));
                if (cred.CredentialBlobSize > 0) {
                    return Marshal.PtrToStringUni(cred.CredentialBlob, cred.CredentialBlobSize / 2);
                }
            } finally {
                CredFree(credPtr);
            }
        }
        return null;
    }
}
"@

$password = [CredentialManager]::GetPassword($target)
if ($password) {
    Write-Output $password
} else {
    exit 1
}
`, target)

	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script) //nolint:gosec // script constructed from constants + hashed base url
	output, err := cmd.Output()
	if err != nil {
		return "", errKeyringNotFound
	}

	encoded := strings.TrimSpace(string(output))
	if encoded == "" {
		return "", errKeyringNotFound
	}

	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return encoded, nil
	}
	return string(data), nil
}

func keyringSaveSecret(account string, secret string) error {
	target := fmt.Sprintf("%s/%s", lesserCLIKeyringServiceName, account)
	encoded := base64.StdEncoding.EncodeToString([]byte(secret))

	script := fmt.Sprintf(`
$target = '%s'
$password = '%s'
cmdkey /generic:$target /user:lesser /pass:$password
`, target, encoded)

	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script) //nolint:gosec // script constructed from constants + hashed base url
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("cmdkey failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
