package discover

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// Leapp keeps its workspace at ~/.Leapp/Leapp-lock.json, AES-encrypted with
// the machine id. This reader imports what maps onto Rolle: Identity Center
// portals, Azure tenants, IAM users, and chained roles.

// LeappIAMUser is an IAM user session from Leapp. Access keys live in the OS
// keychain under the "Leapp" service, and the import reads them.
type LeappIAMUser struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Region    string `json:"region"`
	MFADevice string `json:"mfaDevice,omitempty"`
	Profile   string `json:"profile,omitempty"`
}

// LeappChainedRole is an AssumeRole session whose source is another session.
type LeappChainedRole struct {
	Name       string `json:"name"`
	Region     string `json:"region"`
	RoleARN    string `json:"roleArn"`
	ParentName string `json:"parentName"`
	Profile    string `json:"profile,omitempty"`
}

// LeappWorkspace is the subset of a Leapp workspace Rolle can import.
type LeappWorkspace struct {
	Portals      []AWSPortal        `json:"portals"`
	Tenants      []AzureTenant      `json:"tenants"`
	IAMUsers     []LeappIAMUser     `json:"iamUsers"`
	ChainedRoles []LeappChainedRole `json:"chainedRoles"`
	// SSORoles counts sessions that Rolle rediscovers by syncing the portal.
	SSORoles int `json:"ssoRoles"`
}

// LeappPath returns the Leapp workspace file, honouring LEAPP_HOME for tests.
func LeappPath() string {
	if d := os.Getenv("LEAPP_HOME"); d != "" {
		return filepath.Join(d, "Leapp-lock.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".Leapp", "Leapp-lock.json")
}

// ReadLeapp decrypts and parses the Leapp workspace. It returns nil, nil when
// no Leapp workspace file exists.
func ReadLeapp() (*LeappWorkspace, error) {
	path := LeappPath()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) || path == "" {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	id, err := machineID()
	if err != nil {
		return nil, fmt.Errorf("leapp: %w", err)
	}
	plain, err := decryptCryptoJS(strings.TrimSpace(string(data)), id)
	if err != nil {
		return nil, fmt.Errorf("leapp: decrypt workspace: %w", err)
	}
	return parseLeapp(plain)
}

// leappRaw mirrors the JSON Leapp writes. Field names come from its models.
type leappRaw struct {
	Sessions []struct {
		Type        string `json:"type"`
		SessionName string `json:"sessionName"`
		SessionID   string `json:"sessionId"`
		Region      string `json:"region"`
		ProfileID   string `json:"profileId"`
		RoleARN     string `json:"roleArn"`
		ParentID    string `json:"parentSessionId"`
		MFADevice   string `json:"mfaDevice"`
		SubID       string `json:"subscriptionId"`
		TenantID    string `json:"tenantId"`
	} `json:"_sessions"`
	SSO []struct {
		Alias     string `json:"alias"`
		PortalURL string `json:"portalUrl"`
		Region    string `json:"region"`
	} `json:"_awsSsoIntegrations"`
	Azure []struct {
		Alias    string `json:"alias"`
		TenantID string `json:"tenantId"`
	} `json:"_azureIntegrations"`
	Profiles []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"_profiles"`
}

func parseLeapp(plain []byte) (*LeappWorkspace, error) {
	var raw leappRaw
	if err := json.Unmarshal(plain, &raw); err != nil {
		return nil, fmt.Errorf("leapp: parse workspace: %w", err)
	}
	out := &LeappWorkspace{}
	for _, i := range raw.SSO {
		if i.PortalURL == "" {
			continue
		}
		out.Portals = append(out.Portals, AWSPortal{Alias: i.Alias, StartURL: normalizeStartURL(i.PortalURL), Region: i.Region, Source: "leapp"})
	}
	for _, i := range raw.Azure {
		if i.TenantID != "" {
			out.Tenants = append(out.Tenants, AzureTenant{TenantID: i.TenantID, Account: i.Alias, Source: "leapp"})
		}
	}
	profiles := map[string]string{}
	for _, p := range raw.Profiles {
		profiles[p.ID] = p.Name
	}
	names := map[string]string{}
	for _, s := range raw.Sessions {
		names[s.SessionID] = s.SessionName
	}
	profile := func(id string) string {
		if n := profiles[id]; n != "default" {
			return n
		}
		return ""
	}
	for _, s := range raw.Sessions {
		switch s.Type {
		case "awsIamUser":
			out.IAMUsers = append(out.IAMUsers, LeappIAMUser{ID: s.SessionID, Name: s.SessionName, Region: s.Region, MFADevice: s.MFADevice, Profile: profile(s.ProfileID)})
		case "awsIamRoleChained":
			out.ChainedRoles = append(out.ChainedRoles, LeappChainedRole{Name: s.SessionName, Region: s.Region, RoleARN: s.RoleARN, ParentName: names[s.ParentID], Profile: profile(s.ProfileID)})
		case "awsSsoRole":
			out.SSORoles++
		}
	}
	return out, nil
}

// LeappKeychainKeys are the keychain account names Leapp uses for an IAM user's keys.
func LeappKeychainKeys(sessionID string) (accessKeyID, secret string) {
	return sessionID + "-iam-user-aws-session-access-key-id", sessionID + "-iam-user-aws-session-secret-access-key"
}

// LeappKeychainService is the keychain service name Leapp stores secrets under.
const LeappKeychainService = "Leapp"

// machineID reproduces node-machine-id's machineIdSync(): the platform id,
// lower-cased, SHA-256 hashed and hex encoded.
func machineID() (string, error) {
	raw, err := rawMachineID()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(raw))))
	return hex.EncodeToString(sum[:]), nil
}

var uuidRe = regexp.MustCompile(`"IOPlatformUUID"\s*=\s*"([^"]+)"`)

func rawMachineID() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("ioreg", "-rd1", "-c", "IOPlatformExpertDevice").Output()
		if err != nil {
			return "", err
		}
		m := uuidRe.FindSubmatch(out)
		if m == nil {
			return "", errors.New("IOPlatformUUID not found")
		}
		return string(m[1]), nil
	case "windows":
		out, err := exec.Command("reg", "query", `HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Cryptography`, "/v", "MachineGuid").Output()
		if err != nil {
			return "", err
		}
		f := strings.Fields(string(out))
		if len(f) == 0 {
			return "", errors.New("MachineGuid not found")
		}
		return f[len(f)-1], nil
	default:
		for _, p := range []string{"/var/lib/dbus/machine-id", "/etc/machine-id"} {
			if b, err := os.ReadFile(p); err == nil {
				return strings.TrimSpace(string(b)), nil
			}
		}
		return "", errors.New("machine-id not found")
	}
}

// decryptCryptoJS decrypts the OpenSSL-compatible output of crypto-js
// AES.encrypt(text, passphrase): base64("Salted__" + salt + ciphertext),
// key and iv from EVP_BytesToKey with MD5, AES-256-CBC, PKCS#7 padding.
func decryptCryptoJS(b64, passphrase string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, err
	}
	if len(data) < 16 || string(data[:8]) != "Salted__" {
		return nil, errors.New("not an OpenSSL salted payload")
	}
	salt, body := data[8:16], data[16:]
	key, iv := evpBytesToKey([]byte(passphrase), salt, 32, 16)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(body)%aes.BlockSize != 0 || len(body) == 0 {
		return nil, errors.New("ciphertext is not block aligned")
	}
	plain := make([]byte, len(body))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, body)
	pad := int(plain[len(plain)-1])
	if pad == 0 || pad > aes.BlockSize || pad > len(plain) {
		return nil, errors.New("bad padding: the machine id does not match the workspace")
	}
	// Every pad byte must equal the pad length. A wrong key passes the
	// last-byte check about one time in sixteen; this check catches those.
	for _, b := range plain[len(plain)-pad:] {
		if int(b) != pad {
			return nil, errors.New("bad padding: the machine id does not match the workspace")
		}
	}
	return plain[:len(plain)-pad], nil
}

// encryptCryptoJS encrypts plain in the crypto-js format. Tests use it to build fixtures.
func encryptCryptoJS(plain []byte, passphrase string, salt []byte) string {
	key, iv := evpBytesToKey([]byte(passphrase), salt, 32, 16)
	block, _ := aes.NewCipher(key)
	pad := aes.BlockSize - len(plain)%aes.BlockSize
	padded := append(append([]byte{}, plain...), bytes.Repeat([]byte{byte(pad)}, pad)...)
	out := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(out, padded)
	return base64.StdEncoding.EncodeToString(append(append([]byte("Salted__"), salt...), out...))
}

// evpBytesToKey implements OpenSSL EVP_BytesToKey with MD5 and one iteration.
//
// MD5 is the only choice here. crypto-js derives its AES key this way, and
// Leapp encrypted its workspace with crypto-js, so any other function cannot
// read the file. The passphrase is the machine id, not a user password, and
// Rolle never writes this format for its own data; the encrypt side above
// exists for test fixtures. Rolle's secrets live in the OS keychain.
func evpBytesToKey(pass, salt []byte, keyLen, ivLen int) (key, iv []byte) {
	var d, prev []byte
	for len(d) < keyLen+ivLen {
		h := md5.New()
		h.Write(prev)
		h.Write(pass)
		h.Write(salt)
		prev = h.Sum(nil)
		d = append(d, prev...)
	}
	return d[:keyLen], d[keyLen : keyLen+ivLen]
}
