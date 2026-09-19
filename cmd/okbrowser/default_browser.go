//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows/registry"
)

// default_browser.go registers OK Browser with Windows as a selectable
// default browser and helps the user make it the default.
//
// Windows 10/11 deliberately forbid an app from silently seizing the default
// browser (that must be a user choice in Settings). What an app CAN do is
// publish the registry "Capabilities" that make it appear in the
// Settings > Apps > Default apps list and as an option on http/https links.
// We register under HKEY_CURRENT_USER (no admin rights needed), then open the
// Windows default-apps UI so the user can confirm the choice in one click.

const (
	progID       = "OKBrowserHTML"
	appRegName   = "OKBrowser"
	appFriendly  = "OK Browser"
	capabilities = `Software\OKBrowser\Capabilities`
)

// registerDefaultBrowser writes the HKCU registry entries that let Windows
// offer OK Browser as a default browser. It is idempotent and safe to call
// repeatedly. Returns an error only on a real registry failure.
func registerDefaultBrowser() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	openCmd := `"` + exe + `" "%1"`
	iconRef := exe + ",0"

	// 1. The ProgID: how Windows launches us for an html/http/https doc.
	if err := setKeyValues(`Software\Classes\`+progID, map[string]string{
		"": appFriendly + " Document",
	}); err != nil {
		return err
	}
	if err := setKeyValues(`Software\Classes\`+progID+`\DefaultIcon`, map[string]string{
		"": iconRef,
	}); err != nil {
		return err
	}
	if err := setKeyValues(`Software\Classes\`+progID+`\shell\open\command`, map[string]string{
		"": openCmd,
	}); err != nil {
		return err
	}

	// 2. Application registration + capabilities (drives the Default apps UI).
	if err := setKeyValues(`Software\Classes\Applications\OKBrowser.exe`, map[string]string{
		"FriendlyAppName": appFriendly,
	}); err != nil {
		return err
	}
	if err := setKeyValues(`Software\Classes\Applications\OKBrowser.exe\shell\open\command`, map[string]string{
		"": openCmd,
	}); err != nil {
		return err
	}
	if err := setKeyValues(capabilities, map[string]string{
		"ApplicationName":        appFriendly,
		"ApplicationDescription": "A very light and very fast web browser.",
		"ApplicationIcon":        iconRef,
	}); err != nil {
		return err
	}
	// URL associations: http/https links can open in OK Browser.
	if err := setKeyValues(capabilities+`\URLAssociations`, map[string]string{
		"http":  progID,
		"https": progID,
	}); err != nil {
		return err
	}
	// File associations: .htm/.html files can open in OK Browser.
	if err := setKeyValues(capabilities+`\FileAssociations`, map[string]string{
		".htm":  progID,
		".html": progID,
	}); err != nil {
		return err
	}
	// StartMenuInternet registration marks us as a "web browser" app.
	if err := setKeyValues(`Software\Clients\StartMenuInternet\`+appRegName, map[string]string{
		"": appFriendly,
	}); err != nil {
		return err
	}
	if err := setKeyValues(`Software\Clients\StartMenuInternet\`+appRegName+`\DefaultIcon`, map[string]string{
		"": iconRef,
	}); err != nil {
		return err
	}
	if err := setKeyValues(`Software\Clients\StartMenuInternet\`+appRegName+`\shell\open\command`, map[string]string{
		"": `"` + exe + `"`,
	}); err != nil {
		return err
	}
	if err := setKeyValues(`Software\Clients\StartMenuInternet\`+appRegName+`\Capabilities`, map[string]string{
		"ApplicationName":        appFriendly,
		"ApplicationDescription": "A very light and very fast web browser.",
		"ApplicationIcon":        iconRef,
	}); err != nil {
		return err
	}
	if err := setKeyValues(`Software\Clients\StartMenuInternet\`+appRegName+`\Capabilities\URLAssociations`, map[string]string{
		"http":  progID,
		"https": progID,
	}); err != nil {
		return err
	}
	if err := setKeyValues(`Software\Clients\StartMenuInternet\`+appRegName+`\Capabilities\FileAssociations`, map[string]string{
		".htm":  progID,
		".html": progID,
	}); err != nil {
		return err
	}

	// 3. Announce the capabilities to Windows so we show up in Default apps.
	k, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\RegisteredApplications`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(appRegName, capabilities)
}

// setKeyValues creates (or opens) an HKCU subkey and sets the given values.
// An empty-string key name sets the key's (Default) value.
func setKeyValues(path string, values map[string]string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	for name, value := range values {
		if err := k.SetStringValue(name, value); err != nil {
			return err
		}
	}
	return nil
}

// isDefaultBrowser reports whether OK Browser's ProgID is the current
// user choice for the https UserChoice association.
func isDefaultBrowser() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\Shell\Associations\URLAssociations\https\UserChoice`,
		registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	pid, _, err := k.GetStringValue("ProgId")
	if err != nil {
		return false
	}
	return pid == progID
}

// makeDefaultBrowser registers the app then opens the Windows default-apps
// settings so the user can confirm OK Browser as the default in one click.
// Windows does not allow silently forcing the default, so this is the correct,
// user-respecting flow.
func makeDefaultBrowser() error {
	if err := registerDefaultBrowser(); err != nil {
		return err
	}
	openDefaultAppsSettings()
	return nil
}

// openDefaultAppsSettings opens the modern Settings deep-link to OK Browser's
// default-apps page (Windows 11), falling back to the general defaults page.
func openDefaultAppsSettings() {
	// Windows 11 supports a per-app deep link; the generic page works on all.
	openExternal("ms-settings:defaultapps")
}
