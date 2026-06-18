package ui

import (
	"context"
	"os"
	"path/filepath"

	"github.com/gg582/nozik/internal/ai"
	"github.com/gg582/nozik/internal/config"
	"github.com/gg582/nozik/internal/model"
	"github.com/gotk3/gotk3/gtk"
)

// providerDisplayNames maps internal provider IDs to human-friendly labels.
var providerDisplayNames = map[model.ProviderKind]string{
	model.ProviderGemini: "Gemini",
	model.ProviderOpenAI: "ChatGPT",
	model.ProviderGrok:   "Grok",
	model.ProviderKimi:   "Kimi",
	model.ProviderOther:  "Others (custom Lua)",
}

func (a *App) showSettingsDialog() {
	dlg, err := gtk.DialogNewWithButtons("Nozik Settings", a.window,
		gtk.DIALOG_MODAL|gtk.DIALOG_DESTROY_WITH_PARENT,
		[]interface{}{"_Cancel", gtk.RESPONSE_CANCEL},
		[]interface{}{"_Save", gtk.RESPONSE_OK})
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

	providers := []model.ProviderKind{model.ProviderGemini, model.ProviderOpenAI, model.ProviderGrok, model.ProviderKimi, model.ProviderOther}

	// Provider selection
	providerLabel, _ := gtk.LabelNew("AI Provider")
	providerLabel.SetHAlign(gtk.ALIGN_START)
	grid.Attach(providerLabel, 0, 0, 1, 1)

	providerCombo, _ := gtk.ComboBoxTextNew()
	for _, p := range providers {
		providerCombo.Append(string(p), providerDisplayNames[p])
	}
	if !providerCombo.SetActiveID(string(a.settings.Provider)) {
		providerCombo.SetActiveID(string(model.ProviderGemini))
	}
	grid.Attach(providerCombo, 1, 0, 2, 1)

	activeProvider := func() model.ProviderKind {
		return model.ProviderKind(providerCombo.GetActiveID())
	}

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

	providerModels := map[model.ProviderKind]string{
		model.ProviderGemini: a.settings.GeminiModel,
		model.ProviderOpenAI: a.settings.OpenAIModel,
		model.ProviderGrok:   a.settings.GrokModel,
		model.ProviderKimi:   a.settings.KimiModel,
		model.ProviderOther:  a.settings.Custom.Model,
	}

	currentProvider := activeProvider()
	apiKeyEntry.SetText(providerKeys[currentProvider])
	apiKeyEntry.SetPlaceholderText("Paste your " + providerDisplayNames[currentProvider] + " API key here")

	// Kimi base URL (only relevant for Kimi)
	kimiBaseURLLabel, _ := gtk.LabelNew("Kimi Base URL")
	kimiBaseURLLabel.SetHAlign(gtk.ALIGN_START)
	grid.Attach(kimiBaseURLLabel, 0, 2, 1, 1)

	kimiBaseURLEntry, _ := gtk.EntryNew()
	kimiBaseURLEntry.SetText(a.settings.KimiBaseURL)
	kimiBaseURLEntry.SetPlaceholderText("https://api.moonshot.ai/v1")
	kimiBaseURLEntry.SetHExpand(true)
	grid.Attach(kimiBaseURLEntry, 1, 2, 2, 1)

	updateBaseURLVisibility := func(pk model.ProviderKind) {
		visible := pk == model.ProviderKimi
		kimiBaseURLLabel.SetVisible(visible)
		kimiBaseURLEntry.SetVisible(visible)
	}

	// Model selection
	modelLabel, _ := gtk.LabelNew("Model")
	modelLabel.SetHAlign(gtk.ALIGN_START)
	grid.Attach(modelLabel, 0, 3, 1, 1)

	modelCombo, _ := gtk.ComboBoxTextNewWithEntry()
	modelCombo.SetHExpand(true)
	grid.Attach(modelCombo, 1, 3, 1, 1)

	refreshBtn, _ := gtk.ButtonNewWithLabel("Refresh models")
	grid.Attach(refreshBtn, 2, 3, 1, 1)

	populateModelCombo := func(pk model.ProviderKind) {
		modelCombo.RemoveAll()
		selected := providerModels[pk]
		activeIdx := -1
		for i, m := range a.settings.ModelsCache[pk] {
			modelCombo.AppendText(m.ID)
			if m.ID == selected {
				activeIdx = i
			}
		}
		if activeIdx >= 0 {
			modelCombo.SetActive(activeIdx)
		} else {
			if entry, err := modelCombo.GetEntry(); err == nil {
				entry.SetText(selected)
			}
		}
	}

	setRefreshSensitive := func() {
		refreshBtn.SetSensitive(activeProvider() != model.ProviderOther)
	}

	syncKeyAndModel := func() {
		text, _ := apiKeyEntry.GetText()
		providerKeys[currentProvider] = text
		if entry, err := modelCombo.GetEntry(); err == nil {
			providerModels[currentProvider], _ = entry.GetText()
		}

		currentProvider = activeProvider()
		apiKeyEntry.SetText(providerKeys[currentProvider])
		apiKeyEntry.SetPlaceholderText("Paste your " + providerDisplayNames[currentProvider] + " API key here")
		updateBaseURLVisibility(currentProvider)
		populateModelCombo(currentProvider)
		setRefreshSensitive()
	}

	providerCombo.Connect("changed", syncKeyAndModel)
	populateModelCombo(currentProvider)
	updateBaseURLVisibility(currentProvider)
	setRefreshSensitive()

	refreshBtn.Connect("clicked", func() {
		// Sync the currently-typed API key before reading it.
		syncKeyAndModel()
		pk := activeProvider()
		if pk == model.ProviderOther {
			return
		}
		key := providerKeys[pk]
		if key == "" {
			// Try env fallback for the refresh key.
			switch pk {
			case model.ProviderGemini:
				key = os.Getenv("GEMINI_API_KEY")
			case model.ProviderOpenAI:
				key = os.Getenv("OPENAI_API_KEY")
			case model.ProviderGrok:
				key = os.Getenv("GROK_API_KEY")
			case model.ProviderKimi:
				key = os.Getenv("KIMI_API_KEY")
			}
		}
		if key == "" {
			a.showError("API key is required to refresh models.")
			return
		}

		var lister ai.ModelLister
		switch pk {
		case model.ProviderGemini:
			lister = ai.NewGeminiStrategy(key, "")
		case model.ProviderOpenAI:
			lister = ai.NewOpenAIStrategy(key, "")
		case model.ProviderGrok:
			lister = ai.NewGrokStrategy(key, "")
		case model.ProviderKimi:
			baseURL, _ := kimiBaseURLEntry.GetText()
			lister = ai.NewKimiStrategy(key, "", baseURL)
		}

		models, err := lister.ListModels(context.Background())
		if err != nil {
			a.showError("Failed to refresh models: " + err.Error())
			return
		}
		if a.settings.ModelsCache == nil {
			a.settings.ModelsCache = make(map[model.ProviderKind][]model.ModelInfo)
		}
		a.settings.ModelsCache[pk] = models
		populateModelCombo(pk)
	})

	// Custom script path
	scriptLabel, _ := gtk.LabelNew("Custom Lua Script")
	scriptLabel.SetHAlign(gtk.ALIGN_START)
	grid.Attach(scriptLabel, 0, 4, 1, 1)

	scriptEntry, _ := gtk.EntryNew()
	scriptEntry.SetText(a.settings.Custom.ScriptPath)
	scriptEntry.SetPlaceholderText("/path/to/custom_provider.lua")
	scriptEntry.SetHExpand(true)
	grid.Attach(scriptEntry, 1, 4, 1, 1)

	browseBtn, _ := gtk.ButtonNewWithLabel("Browse")
	browseBtn.Connect("clicked", func() {
		chooser, _ := gtk.FileChooserDialogNewWith2Buttons("Select Lua Script", dlg,
			gtk.FILE_CHOOSER_ACTION_OPEN, "_Cancel", gtk.RESPONSE_CANCEL, "_Open", gtk.RESPONSE_ACCEPT)
		filter, _ := gtk.FileFilterNew()
		filter.AddPattern("*.lua")
		filter.SetName("Lua scripts")
		chooser.AddFilter(filter)
		if chooser.Run() == gtk.RESPONSE_ACCEPT {
			scriptEntry.SetText(chooser.GetFilename())
		}
		chooser.Destroy()
	})
	grid.Attach(browseBtn, 2, 4, 1, 1)

	// API key hint label
	hint, _ := gtk.LabelNew("API keys are saved in settings.json. If left blank, the matching *_API_KEY environment variable is used as a fallback. For Kimi, use Base URL to switch between api.moonshot.ai (international) and api.moonshot.cn (China).")
	hint.SetLineWrap(true)
	hint.SetProperty("name", "AlbumArtist")
	grid.Attach(hint, 0, 5, 3, 1)

	// DB actions
	sep, _ := gtk.SeparatorNew(gtk.ORIENTATION_HORIZONTAL)
	grid.Attach(sep, 0, 6, 3, 1)

	exportBtn, _ := gtk.ButtonNewWithLabel("Export Taste DB")
	exportBtn.Connect("clicked", func() {
		a.onExportTaste(dlg)
	})
	grid.Attach(exportBtn, 0, 7, 1, 1)

	importBtn, _ := gtk.ButtonNewWithLabel("Import Taste DB")
	importBtn.Connect("clicked", func() {
		a.onImportTaste(dlg)
	})
	grid.Attach(importBtn, 1, 7, 1, 1)

	joinBtn, _ := gtk.ButtonNewWithLabel("Join Taste DB")
	joinBtn.Connect("clicked", func() {
		a.onJoinTaste(dlg)
	})
	grid.Attach(joinBtn, 2, 7, 1, 1)

	clearBtn, _ := gtk.ButtonNewWithLabel("Clear Local DB")
	clearBtn.Connect("clicked", func() {
		a.onClearLocalDB()
	})
	grid.Attach(clearBtn, 0, 8, 1, 1)

	content.ShowAll()
	resp := dlg.Run()
	if resp == gtk.RESPONSE_OK {
		syncKeyAndModel()
		a.settings.Provider = activeProvider()
		a.settings.GeminiKey = providerKeys[model.ProviderGemini]
		a.settings.OpenAIKey = providerKeys[model.ProviderOpenAI]
		a.settings.GrokKey = providerKeys[model.ProviderGrok]
		a.settings.KimiKey = providerKeys[model.ProviderKimi]
		a.settings.GeminiModel = providerModels[model.ProviderGemini]
		a.settings.OpenAIModel = providerModels[model.ProviderOpenAI]
		a.settings.GrokModel = providerModels[model.ProviderGrok]
		a.settings.KimiModel = providerModels[model.ProviderKimi]
		a.settings.KimiBaseURL, _ = kimiBaseURLEntry.GetText()
		path, _ := scriptEntry.GetText()
		a.settings.Custom = ai.LuaCustomConfig{
			Name:       "Others",
			ScriptPath: path,
			APIKey:     a.settings.Custom.APIKey,
			Model:      providerModels[model.ProviderOther],
			Defaults:   a.settings.Custom.Defaults,
		}
		if err := a.saveSettings(); err != nil {
			a.showError("Failed to save settings: " + err.Error())
		}
	}
}

func (a *App) onExportTaste(parent *gtk.Dialog) {
	if a.db == nil {
		a.showError("Local DB is not available.")
		return
	}
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
	if a.db == nil {
		a.showError("Local DB is not available.")
		return
	}
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
	if a.db == nil {
		a.showError("Local DB is not available.")
		return
	}
	chooser, _ := gtk.FileChooserDialogNewWith2Buttons("Join Taste DB", parent,
		gtk.FILE_CHOOSER_ACTION_OPEN, "_Cancel", gtk.RESPONSE_CANCEL, "_Open", gtk.RESPONSE_ACCEPT)
	if chooser.Run() == gtk.RESPONSE_ACCEPT {
		if err := a.db.Join(chooser.GetFilename()); err != nil {
			a.showError(err.Error())
		}
	}
	chooser.Destroy()
}

func (a *App) onClearLocalDB() {
	dbPath := config.DBPath()
	if err := os.Remove(dbPath); err != nil {
		if os.IsNotExist(err) {
			a.showError("Local taste DB is already empty. Restart Nozik for a clean install.")
			return
		}
		a.showError("Failed to clear local DB: " + err.Error())
		return
	}
	a.showError("Local taste DB cleared. Please restart Nozik to complete a clean install.")
}

func (a *App) showError(msg string) {
	dlg := gtk.MessageDialogNew(a.window, gtk.DIALOG_MODAL|gtk.DIALOG_DESTROY_WITH_PARENT,
		gtk.MESSAGE_ERROR, gtk.BUTTONS_OK, "%s", msg)
	dlg.Run()
	dlg.Destroy()
}

// AssetPath is declared in app.go; this file uses it implicitly.
var _ = filepath.Join
