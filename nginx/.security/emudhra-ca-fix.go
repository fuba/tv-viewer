// SPDX-License-Identifier: MIT
// This tool removes only fingerprint-matched compromised CA trust paths.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
)

var blocked = map[string]bool{
	"86a1ecba089c4a8d3bbe2734c612ba341d813e043cf9e8a862cd5c57a36bbe6b": true,
	"d7a7a0fb5d7e2731d771e9484ebcdef71d5f0c3e0a2948782bc83ee0ea699ef4": true,
	"7dd78d5f4f13459a83dff9abbba62edbaf6f2d102bf257fd712f4d9f2746ed8d": true,
	"f2edaaff3d9b701fb4d14c9f285f8c0bad90fc3bcc1fa07d47f4718edaba34a5": true,
}
var certificate = regexp.MustCompile(`(?s)-----BEGIN CERTIFICATE-----\s+.*?-----END CERTIFICATE-----\r?\n?`)

func filter(data []byte, blocklist map[string]bool) ([]byte, int) {
	count := 0
	result := certificate.ReplaceAllFunc(data, func(raw []byte) []byte {
		block, _ := pem.Decode(raw)
		if block == nil || block.Type != "CERTIFICATE" {
			return raw
		}
		digest := sha256.Sum256(block.Bytes)
		if blocklist[hex.EncodeToString(digest[:])] {
			count++
			return nil
		}
		return raw
	})
	return result, count
}

func replace(path string, data []byte, info fs.FileInfo) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".emudhra-*")
	if err != nil {
		return err
	}
	tmp := file.Name()
	defer os.Remove(tmp)
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	if err = os.Chmod(tmp, info.Mode().Perm()); err != nil {
		return err
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		if err = os.Chown(tmp, int(stat.Uid), int(stat.Gid)); err != nil {
			return err
		}
	}
	return os.Rename(tmp, path)
}

func mergeJavaBlocklist(data []byte) ([]byte, error) {
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	seen := map[string]bool{}
	for _, line := range lines {
		seen[line] = true
	}
	if !seen["Algorithm=SHA-256"] {
		return nil, fmt.Errorf("unsupported JDK blocklist algorithm")
	}
	for _, line := range strings.Split(javaBlocklist, "\n")[1:] {
		if !seen[line] {
			lines = append(lines, line)
		}
	}
	return []byte(strings.Join(lines, "\n") + "\n"), nil
}

const javaBlocklist = `Algorithm=SHA-256
0164D7CEAE5BB909C272AF151FF44B1177AE9B47D9424B94F1D9558C30934D19
42C4ACCD12C4F59597925E1A08A7A90102608543809308D50EC427E118BA1493
432EC25F0D28DE9006D97BF752F17BBB2CD41F669933FCB072D9A1148D0261F3
45A883F349AEDFD5F3A649B5498FFB121712BA8B448F3F2A91DBC5A766FCDDBE
4B2460A466F95FD40113BCDCCA2CEB29B3B093921DE45474DA7DF48874C630B1
579480AEA166290BD3C643589E795FC0D0E54E86505D180F1DFF99EFD3199512
5F1DED4DD590AF74631290CF073B122A2AAC6A1FA479778A0BD43579BD832699
62B7F9868263F72F6911FC78CEAF72C1979158B7368BF648683BB2AA39B02E72
7485B51C0ED192878FA1D2F948FB529AE1842CC5086C4B67F7BE5B2039DE9489
84112972B8E59D14560AE7E7119654474AAD119990CF3722BFC1E3B91615E1FD
98B896625C0C230ADEEC3EB2CF6AAD5001ECF9156A68644E82374CE5833BC25B
9E39DE2881273ACC225A149C72BA3E0F51424963669AB633EE1DC7DF5DB067B4
A496E23AC0CAB8155FC9B117B97EAE368EFCBB9041AEC8804D5A308D2D01C2D8
CB6E15165AC802E9B3F993F16C25D41F4E81C21EBDE71AE1B012AC00BF9C7C3F
E7568FE6E12331F4FD7DE61DA4C7559D0F8EB192A218D4741A3BA75184333838
FD9D5C2DBBD4995AB69B8D50C439D9527B00529199C2FD01B2174EB58F07021F`

func main() {
	if os.Geteuid() != 0 {
		fmt.Fprintln(os.Stderr, "Run inside the container as UID 0")
		os.Exit(1)
	}
	if len(os.Args) > 1 && os.Args[1] == "--self-delete" {
		if path, err := os.Executable(); err == nil {
			os.Remove(path)
		}
	}
	changed, checked := []string{}, 0
	failures := []string{}
	// Keep future update-ca-certificates runs from restoring the two trust roots.
	if data, err := os.ReadFile("/etc/ca-certificates.conf"); err == nil {
		lines := strings.Split(string(data), "\n")
		for index, line := range lines {
			if line == "mozilla/emSign_ECC_Root_CA_-_G3.crt" || line == "mozilla/Comodo_AAA_Services_root.crt" {
				lines[index] = "!" + line
			}
		}
		filtered := strings.Join(lines, "\n")
		if filtered != string(data) {
			if info, err := os.Stat("/etc/ca-certificates.conf"); err == nil {
				if err := replace("/etc/ca-certificates.conf", []byte(filtered), info); err != nil {
					failures = append(failures, err.Error())
				}
			}
		}
	}
	roots := []string{"/etc/ssl", "/etc/pki", "/usr/share/ca-certificates", "/usr/local/share/ca-certificates", "/usr/local/lib", "/usr/lib", "/opt", "/app", "/fuba_recorder/local", "/root/fuba_recorder/local", "/ak-root/.venv", "/home/dev/.local/lib"}
	seen := map[string]bool{}
	for _, root := range roots {
		filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if entry.IsDir() {
				return nil
			}
			if !strings.HasPrefix(path, "/etc/") && !strings.HasSuffix(path, ".pem") && !strings.HasSuffix(path, ".crt") {
				return nil
			}
			actual, err := filepath.EvalSymlinks(path)
			if err != nil || seen[path] {
				return nil
			}
			seen[path] = true
			info, err := os.Stat(actual)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 4_000_000 {
				return nil
			}
			data, err := os.ReadFile(actual)
			if err != nil || bytes.Contains(data, []byte("PRIVATE KEY-----")) || !certificate.Match(data) {
				return nil
			}
			checked++
			filtered, count := filter(data, blocked)
			if count == 0 {
				return nil
			}
			if certificate.Match(filtered) {
				err = replace(actual, filtered, info)
			} else {
				err = os.Remove(path)
			}
			if err != nil {
				failures = append(failures, path+": "+err.Error())
			} else {
				changed = append(changed, path)
			}
			return nil
		})
	}
	// Java uses its own certificate verifier and a binary trust store.
	for _, pattern := range []string{"/usr/lib/jvm/*/lib/security/blocked.certs", "/usr/lib/jvm/*/jre/lib/security/blacklisted.certs"} {
		paths, _ := filepath.Glob(pattern)
		for _, path := range paths {
			actual, err := filepath.EvalSymlinks(path)
			if err != nil || seen[actual] {
				continue
			}
			seen[actual] = true
			data, err := os.ReadFile(actual)
			if err != nil {
				failures = append(failures, path+": "+err.Error())
				continue
			}
			filtered, err := mergeJavaBlocklist(data)
			if err == nil && !bytes.Equal(data, filtered) {
				info, statErr := os.Stat(actual)
				if statErr != nil {
					err = statErr
				} else {
					err = replace(actual, filtered, info)
				}
				if err == nil {
					changed = append(changed, actual)
				}
			}
			if err != nil {
				failures = append(failures, path+": "+err.Error())
			}
		}
	}
	json.NewEncoder(os.Stdout).Encode(map[string]interface{}{"checked": checked, "changed": changed, "errors": failures})
	if len(failures) > 0 {
		os.Exit(2)
	}
}
