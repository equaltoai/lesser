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
`, powershellSingleQuoted(target))

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
$password = [Console]::In.ReadToEnd().Trim()
if ([string]::IsNullOrWhiteSpace($password)) {
    exit 2
}

Add-Type @"
using System;
using System.ComponentModel;
using System.Runtime.InteropServices;
using System.Text;

public class CredentialWriter {
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

    [DllImport("advapi32.dll", SetLastError = true, CharSet = CharSet.Unicode)]
    public static extern bool CredWrite(ref CREDENTIAL userCredential, uint flags);

    public static void SavePassword(string target, string userName, string password) {
        byte[] bytes = Encoding.Unicode.GetBytes(password);
        IntPtr blob = Marshal.AllocHGlobal(bytes.Length);
        try {
            Marshal.Copy(bytes, 0, blob, bytes.Length);
            CREDENTIAL credential = new CREDENTIAL();
            credential.Type = 1;
            credential.TargetName = target;
            credential.UserName = userName;
            credential.CredentialBlobSize = bytes.Length;
            credential.CredentialBlob = blob;
            credential.Persist = 2;
            if (!CredWrite(ref credential, 0)) {
                throw new Win32Exception(Marshal.GetLastWin32Error());
            }
        } finally {
            Array.Clear(bytes, 0, bytes.Length);
            for (int i = 0; i < bytes.Length; i++) {
                Marshal.WriteByte(blob, i, 0);
            }
            Marshal.FreeHGlobal(blob);
        }
    }
}
"@

[CredentialWriter]::SavePassword($target, "lesser", $password)
`, powershellSingleQuoted(target))

	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script) //nolint:gosec // script constructed from constants + hashed base url
	cmd.Stdin = strings.NewReader(encoded)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("credential write failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func powershellSingleQuoted(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}
