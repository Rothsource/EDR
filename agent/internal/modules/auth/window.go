//go:build windows

package auth

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"sort"
	"strings"

	"khemstrix-agent/internal/event"
)

const DefaultBookmarkFile = `%ProgramData%\khemstrix-agent\auth_bookmark.xml`
const DefaultAuthConfigFileWindows = `%ProgramData%\khemstrix-agent\config\auth.json`

var windowsEventIDs = map[string]uint16{
	"windows-4624": 4624, "windows-4625": 4625, "windows-4634": 4634,
	"windows-4647": 4647, "windows-4648": 4648, "windows-4672": 4672,
	"windows-4778": 4778, "windows-4779": 4779, "windows-4800": 4800,
	"windows-4801": 4801, "windows-4776": 4776, "windows-4768": 4768,
	"windows-4769": 4769, "windows-4771": 4771, "windows-4720": 4720,
	"windows-4722": 4722, "windows-4723": 4723, "windows-4724": 4724,
	"windows-4725": 4725, "windows-4726": 4726, "windows-4740": 4740,
	"windows-4767": 4767, "windows-4728": 4728, "windows-4732": 4732,
	"windows-4756": 4756,
}

type WindowsReaderOptions struct {
	BookmarkFile string
	ConfigFile   string
	AgentID      string
	Push         func(eventID string, payload []byte) error
}

func buildXPath(enabled map[string]bool) string {
	var ids []int
	for name, id := range windowsEventIDs {
		if enabled[name] {
			ids = append(ids, int(id))
		}
	}
	if len(ids) == 0 {
		return ""
	}
	sort.Ints(ids)

	const perSelect = 20
	var b strings.Builder
	b.WriteString(`<QueryList><Query Id="0" Path="Security">`)
	for start := 0; start < len(ids); start += perSelect {
		end := start + perSelect
		if end > len(ids) {
			end = len(ids)
		}
		parts := make([]string, 0, end-start)
		for _, id := range ids[start:end] {
			parts = append(parts, fmt.Sprintf("EventID=%d", id))
		}
		fmt.Fprintf(&b, `<Select Path="Security">*[System[(%s)]]</Select>`, strings.Join(parts, " or "))
	}
	b.WriteString(`</Query></QueryList>`)
	return b.String()
}

func ReadSecurityLog(ctx context.Context, opts WindowsReaderOptions) error {
	if opts.BookmarkFile == "" {
		opts.BookmarkFile = DefaultBookmarkFile
	}
	if opts.ConfigFile == "" {
		opts.ConfigFile = DefaultAuthConfigFileWindows
	}
	if opts.Push == nil {
		return fmt.Errorf("auth(windows): ReaderOptions.Push is required")
	}

	enabled, err := loadWindowsAuthConfig(opts.ConfigFile)
	if err != nil {
		return fmt.Errorf("auth(windows): loading config: %w", err)
	}
	xpath := buildXPath(enabled)
	if xpath == "" {
		return fmt.Errorf("auth(windows): no sources enabled, nothing to watch")
	}

	return runWithBackoff(ctx, xpath, opts.BookmarkFile, func(xmlEvent string) error {
		return handleEvent(xmlEvent, opts)
	})
}

func handleEvent(xmlEvent string, opts WindowsReaderOptions) error {
	var raw struct {
		System struct {
			EventID     uint16 `xml:"EventID"`
			TimeCreated struct {
				SystemTime string `xml:"SystemTime,attr"`
			} `xml:"TimeCreated"`
			EventRecordID uint64 `xml:"EventRecordID"`
			Channel       string `xml:"Channel"`
		} `xml:"System"`
		EventData struct {
			Data []struct {
				Name  string `xml:"Name,attr"`
				Value string `xml:",chardata"`
			} `xml:"Data"`
		} `xml:"EventData"`
	}
	if err := xml.Unmarshal([]byte(xmlEvent), &raw); err != nil {
		return fmt.Errorf("auth(windows): parsing event XML: %w", err)
	}

	fields := map[string]string{}
	for _, d := range raw.EventData.Data {
		fields[d.Name] = d.Value
	}
	fields["EventID"] = fmt.Sprint(raw.System.EventID)
	fields["RecordID"] = fmt.Sprint(raw.System.EventRecordID)

	eventID := event.DeterministicID(
		opts.AgentID, "windows-security", raw.System.Channel,
		fmt.Sprint(raw.System.EventRecordID), raw.System.TimeCreated.SystemTime,
	)

	id, payload, err := event.Build(event.Params{
		ClassUID:    placeholderClassUID,
		CategoryUID: placeholderCategoryUID,
		ActivityID:  placeholderActivityID,
		SeverityID:  placeholderSeverityID,
		EventID:     eventID,
		Time:        parseWindowsTime(raw.System.TimeCreated.SystemTime),
		Data: map[string]any{
			"source": "windows-security",
			"raw":    fields,
		},
	})
	if err != nil {
		return fmt.Errorf("auth(windows): build event: %w", err)
	}

	if err := opts.Push(id, payload); err != nil {
		return fmt.Errorf("auth(windows): push failed, will retry: %w", err)
	}

	return nil
}

func loadWindowsAuthConfig(path string) (map[string]bool, error) {
	enabled := map[string]bool{"windows-4624": true, "windows-4625": true}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return enabled, nil
		}
		return nil, err
	}
	var cfg authConfig
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}
	result := map[string]bool{}
	for _, src := range cfg.Sources {
		if _, ok := windowsEventIDs[src]; ok {
			result[src] = true
		}
	}
	if len(result) == 0 {
		return enabled, nil
	}
	return result, nil
}
