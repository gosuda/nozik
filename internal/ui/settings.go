package ui

import (
	"os"
	"path/filepath"

	"github.com/gg582/nozik/internal/ai"
	"github.com/gg582/nozik/internal/model"
	"github.com/gotk3/gotk3/gtk"
)

func (a *App) showSettingsDialog() {
	dlg, err := gtk.DialogNewWithButtons("Nozik Settings", a.window,
		gtk.DIALOG_MODAL|gtk.DIALOG_DESTROY_WITH_PARENT,
		[]interface{}{"_Cancel", gtk.RESPONSE_CANCEL, "_Save", gtk.RESPONSE_OK})
	if err != nil {
		return
	}
	defer dlg.Destroy()
	content, _ := dlg.GetContentArea()
	content.SetMarginTop(12)
	content.SetMarginBottom(12)
	content.SetMarginStart(12)
	content.SetMarginEnd(12)

	grid, _ := gtk.GridNew()
	grid.SetRowSpacing(8)
	grid.SetColumnSpacing(12)
	grid.SetRowHomogeneous(false)
	content.PackStart(grid, true, true, 0)

	// Provider selection
	providerLabel, _ := gtk.LabelNew("AI Provider")
	providerLabel.SetHAlign(gtk.ALIGN_START)
	grid.Attach(providerLabel, 0, 0, 1, 1)

	providerCombo, _ := gtk.ComboBoxTextNew()
	providers := []model.ProviderKind{model.ProviderGemini, model.ProviderOpenAI, model.ProviderGrok, model.ProviderKimi, model.ProviderOther}
	for _, p := range providers {
		providerCombo.AppendText(string(p))
	}
	providerCombo.SetActive(int(indexOf(providers, a.settings.Provider)))
	grid.Attach(providerCombo, 1, 0, 1, 1)

	// API key entry
	keyLabel, _ := gtk.LabelNew("API Key")
	keyLabel.SetHAlign(gtk.ALIGN_START)
	grid.Attach(keyLabel, 0, 1, 1, 1)

	apiKeyEntry, _ := gtk.EntryNew()
	apiKeyEntry.SetVisibility(false)
	apiKeyEntry.SetHExpand(true)
	grid.Attach(apiKeyEntry, 1, 1, 2, 1)

	providerKeys := map[model.ProviderKind]string{
		model.ProviderGemini: a.settings.GeminiKey,
		model.ProviderOpenAI: a.settings.OpenAIKey,
		model.ProviderGrok:   a.settings.GrokKey,
		model.ProviderKimi:   a.settings.KimiKey,
	}
	currentProvider := providers[providerCombo.GetActive()]
	apiKeyEntry.SetText(providerKeys[currentProvider])
	apiKeyEntry.SetPlaceholderText("Paste your " + string(currentProvider) + " API key here")

	syncKeyEntry := func() {
		text, _ := apiKeyEntry.GetText()
		providerKeys[currentProvider] = text
		currentProvider = providers[providerCombo.GetActive()]
		apiKeyEntry.SetText(providerKeys[currentProvider])
		apiKeyEntry.SetPlaceholderText("Paste your " + string(currentProvider) + " API key here")
	}
	providerCombo.Connect("changed", syncKeyEntry)

	// Custom script path
	scriptLabel, _ := gtk.LabelNew("Custom Luau Script")
	scriptLabel.SetHAlign(gtk.ALIGN_START)
	grid.Attach(scriptLabel, 0, 2, 1, 1)

	scriptEntry, _ := gtk.EntryNew()
	scriptEntry.SetText(a.settings.Custom.ScriptPath)
	scriptEntry.SetPlaceholderText("/path/to/custom_provider.lua")
	scriptEntry.SetHExpand(true)
	grid.Attach(scriptEntry, 1, 2, 1, 1)

	browseBtn, _ := gtk.ButtonNewWithLabel("Browse")
	browseBtn.Connect("clicked", func() {
		chooser, _ := gtk.FileChooserDialogNewWith2Buttons("Select Luau Script", dlg,
			gtk.FILE_CHOOSER_ACTION_OPEN, "_Cancel", gtk.RESPONSE_CANCEL, "_Open", gtk.RESPONSE_ACCEPT)
		filter, _ := gtk.FileFilterNew()
		filter.AddPattern("*.lua")
		filter.SetName("Luau scripts")
		chooser.AddFilter(filter)
		if chooser.Run() == gtk.RESPONSE_ACCEPT {
			scriptEntry.SetText(chooser.GetFilename())
		}
		chooser.Destroy()
	})
	grid.Attach(browseBtn, 2, 2, 1, 1)

	// API key hint label
	hint, _ := gtk.LabelNew("API keys are saved in settings.json. If left blank, the matching *_API_KEY environment variable is used as a fallback.")
	hint.SetLineWrap(true)
	hint.SetProperty("name", "AlbumArtist")
	grid.Attach(hint, 0, 3, 3, 1)

	// DB actions
	sep, _ := gtk.SeparatorNew(gtk.ORIENTATION_HORIZONTAL)
	grid.Attach(sep, 0, 4, 3, 1)

	exportBtn, _ := gtk.ButtonNewWithLabel("Export Taste DB")
	exportBtn.Connect("clicked", func() {
		a.onExportTaste(dlg)
	})
	grid.Attach(exportBtn, 0, 5, 1, 1)

	importBtn, _ := gtk.ButtonNewWithLabel("Import Taste DB")
	importBtn.Connect("clicked", func() {
		a.onImportTaste(dlg)
	})
	grid.Attach(importBtn, 1, 5, 1, 1)

	joinBtn, _ := gtk.ButtonNewWithLabel("Join Taste DB")
	joinBtn.Connect("clicked", func() {
		a.onJoinTaste(dlg)
	})
	grid.Attach(joinBtn, 2, 5, 1, 1)

	content.ShowAll()
	resp := dlg.Run()
	if resp == gtk.RESPONSE_OK {
		syncKeyEntry()
		a.settings.Provider = providers[providerCombo.GetActive()]
		a.settings.GeminiKey = providerKeys[model.ProviderGemini]
		a.settings.OpenAIKey = providerKeys[model.ProviderOpenAI]
		a.settings.GrokKey = providerKeys[model.ProviderGrok]
		a.settings.KimiKey = providerKeys[model.ProviderKimi]
		path, _ := scriptEntry.GetText()
		a.settings.Custom = ai.LuauCustomConfig{
			Name:       "Others",
			ScriptPath: path,
		}
		if err := a.saveSettings(); err != nil {
			a.showError("Failed to save settings: " + err.Error())
		}
	}
}

func (a *App) onExportTaste(parent *gtk.Dialog) {
	chooser, _ := gtk.FileChooserDialogNewWith2Buttons("Export Taste DB", parent,
		gtk.FILE_CHOOSER_ACTION_SAVE, "_Cancel", gtk.RESPONSE_CANCEL, "_Save", gtk.RESPONSE_ACCEPT)
	chooser.SetCurrentName("nozik-taste.json")
	if chooser.Run() == gtk.RESPONSE_ACCEPT {
		f, err := os.Create(chooser.GetFilename())
		if err != nil {
			a.showError(err.Error())
		} else {
			defer f.Close()
			if err := a.db.Export(f); err != nil {
				a.showError(err.Error())
			}
		}
	}
	chooser.Destroy()
}

func (a *App) onImportTaste(parent *gtk.Dialog) {
	chooser, _ := gtk.FileChooserDialogNewWith2Buttons("Import Taste DB", parent,
		gtk.FILE_CHOOSER_ACTION_OPEN, "_Cancel", gtk.RESPONSE_CANCEL, "_Open", gtk.RESPONSE_ACCEPT)
	if chooser.Run() == gtk.RESPONSE_ACCEPT {
		f, err := os.Open(chooser.GetFilename())
		if err != nil {
			a.showError(err.Error())
		} else {
			defer f.Close()
			if err := a.db.Import(f); err != nil {
				a.showError(err.Error())
			}
		}
	}
	chooser.Destroy()
}

func (a *App) onJoinTaste(parent *gtk.Dialog) {
	chooser, _ := gtk.FileChooserDialogNewWith2Buttons("Join Taste DB", parent,
		gtk.FILE_CHOOSER_ACTION_OPEN, "_Cancel", gtk.RESPONSE_CANCEL, "_Open", gtk.RESPONSE_ACCEPT)
	if chooser.Run() == gtk.RESPONSE_ACCEPT {
		if err := a.db.Join(chooser.GetFilename()); err != nil {
			a.showError(err.Error())
		}
	}
	chooser.Destroy()
}

func (a *App) showError(msg string) {
	dlg := gtk.MessageDialogNew(a.window, gtk.DIALOG_MODAL|gtk.DIALOG_DESTROY_WITH_PARENT,
		gtk.MESSAGE_ERROR, gtk.BUTTONS_OK, "%s", msg)
	dlg.Run()
	dlg.Destroy()
}

func indexOf(providers []model.ProviderKind, target model.ProviderKind) int {
	for i, p := range providers {
		if p == target {
			return i
		}
	}
	return 0
}

// AssetPath is declared in app.go; this file uses it implicitly.
var _ = filepath.Join
