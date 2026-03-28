package types

const (
	ItemTypeLogin      = 1
	ItemTypeSecureNote = 2
	ItemTypeCard       = 3
	ItemTypeIdentity   = 4
	ItemTypeSSHKey     = 5
)

type Status struct {
	ServerURL string `json:"serverUrl"`
	LastSync  string `json:"lastSync"`
	UserEmail string `json:"userEmail"`
	UserID    string `json:"userId"`
	Status    string `json:"status"`
}

type Folder struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Item struct {
	ID       string        `json:"id"`
	FolderID string        `json:"folderId"`
	Type     int           `json:"type"`
	Name     string        `json:"name"`
	Notes    string        `json:"notes"`
	Fields   []CustomField `json:"fields"`
	SSHKey   *SSHKeyData   `json:"sshKey,omitempty"`
}

type CustomField struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Type  int    `json:"type"`
}

type SSHKeyData struct {
	PrivateKey string `json:"privateKey"`
	PublicKey  string `json:"publicKey"`
	KeyFingerprint string `json:"keyFingerprint"`
}

// GetCustomField returns the value of a custom field by name, or empty string if not found.
func (item *Item) GetCustomField(name string) string {
	for _, f := range item.Fields {
		if f.Name == name {
			return f.Value
		}
	}
	return ""
}

// GetHosts extracts host entries from the item's Notes field.
// Hosts are expected to be one per line in the notes.
func (item *Item) GetHosts() []string {
	if item.Notes == "" {
		return nil
	}
	var hosts []string
	for _, line := range splitLines(item.Notes) {
		line = trimSpace(line)
		if line != "" {
			hosts = append(hosts, line)
		}
	}
	return hosts
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}
