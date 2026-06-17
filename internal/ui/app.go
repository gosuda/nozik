package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/gg582/nozik/internal/ai"
	"github.com/gg582/nozik/internal/config"
	"github.com/gg582/nozik/internal/curation"
	"github.com/gg582/nozik/internal/db"
	"github.com/gg582/nozik/internal/gzk"
	"github.com/gg582/nozik/internal/model"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/gtk"
)

// AssetPath resolves a bundled asset file.
func AssetPath(name string) string {
	if dir := os.Getenv("NOZIK_ASSETS_DIR"); dir != "" {
		return filepath.Join(dir, name)
	}
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), "assets", name)
	}
	return filepath.Join("assets", name)
}

// App is the top-level GTK application.
type App struct {
	application *gtk.Application
	window      *gtk.ApplicationWindow
	db          *db.DB
	settings    Settings
	gozik       *gzk.Client

	// UI widgets
	chatView        *gtk.TextView
	chatBuffer      *gtk.TextBuffer
	entry           *gtk.Entry
	sendBtn         *gtk.Button
	recBox          *gtk.Box
	actionBox       *gtk.Box
	approveBtn      *gtk.Button
	modifyBtn       *gtk.Button
	denyBtn         *gtk.Button
	songlistBox     *gtk.ListBox
	songlistScroll  *gtk.ScrolledWindow
	statusLabel     *gtk.Label

	selectedRec *model.Recommendation
	pendingRecs []model.Recommendation
}

// Settings is the persisted UI configuration.
type Settings struct {
	Provider   model.ProviderKind  `json:"provider"`
	GeminiKey  string              `json:"gemini_key,omitempty"`
	OpenAIKey  string              `json:"openai_key,omitempty"`
	GrokKey    string              `json:"grok_key,omitempty"`
	KimiKey    string              `json:"kimi_key,omitempty"`
	Custom     ai.LuauCustomConfig `json:"custom"`
}

// NewApp creates the Nozik GTK app.
func NewApp(database *db.DB) (*App, error) {
	app, err := gtk.ApplicationNew("com.gosuda.nozik.agent", glib.APPLICATION_FLAGS_NONE)
	if err != nil {
		return nil, err
	}
	a := &App{
		application: app,
		db:          database,
		settings:    Settings{Provider: model.ProviderGemini},
	}
	_ = a.loadSettings()
	app.Connect("activate", a.onActivate)
	return a, nil
}

// Run starts the GTK main loop.
func (a *App) Run() int {
	return a.application.Run(os.Args)
}

func (a *App) onActivate() {
	if a.window != nil {
		a.window.Present()
		return
	}
	win, err := gtk.ApplicationWindowNew(a.application)
	if err != nil {
		log.Fatal(err)
	}
	a.window = win
	win.SetTitle("Nozik")
	win.SetDefaultSize(900, 760)
	win.SetPosition(gtk.WIN_POS_CENTER)

	if iconPath := AssetPath(filepath.Join("icons", "nozik.png")); fileExists(iconPath) {
		if pix, err := gdk.PixbufNewFromFile(iconPath); err == nil {
			win.SetIcon(pix)
		}
	}

	a.applyCSS()

	// Header
	hb, _ := gtk.HeaderBarNew()
	hb.SetShowCloseButton(true)
	hb.SetTitle("Nozik")
	hb.SetSubtitle("Ask Nozik. Then Play Music.")
	win.SetTitlebar(hb)

	settingsBtn, _ := gtk.ButtonNewFromIconName("emblem-system-symbolic", gtk.ICON_SIZE_BUTTON)
	settingsBtn.SetTooltipText("Settings")
	settingsBtn.Connect("clicked", func() { a.showSettingsDialog() })
	hb.PackEnd(settingsBtn)

	// Main vertical box
	vbox, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 12)
	vbox.SetMarginTop(18)
	vbox.SetMarginBottom(18)
	vbox.SetMarginStart(18)
	vbox.SetMarginEnd(18)
	win.Add(vbox)

	// Chat view
	scroll, _ := gtk.ScrolledWindowNew(nil, nil)
	scroll.SetPolicy(gtk.POLICY_AUTOMATIC, gtk.POLICY_AUTOMATIC)
	scroll.SetMinContentHeight(320)
	scroll.SetProperty("name", "SonglistScroll")
	a.chatView, _ = gtk.TextViewNew()
	a.chatView.SetEditable(false)
	a.chatView.SetWrapMode(gtk.WRAP_WORD_CHAR)
	a.chatView.SetProperty("name", "LyricsScroll")
	a.chatBuffer, _ = a.chatView.GetBuffer()
	a.appendChat("Nozik", "Hi, which song would you like to play?")
	scroll.Add(a.chatView)
	vbox.PackStart(scroll, true, true, 0)

	// Input row
	inputBox, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 8)
	a.entry, _ = gtk.EntryNew()
	a.entry.SetPlaceholderText("Type a request...")
	a.entry.SetHExpand(true)
	a.entry.Connect("activate", func() { a.onSend() })
	a.sendBtn, _ = gtk.ButtonNewWithLabel("Send")
	a.sendBtn.SetProperty("name", "BtnOpen")
	a.sendBtn.Connect("clicked", func() { a.onSend() })
	inputBox.PackStart(a.entry, true, true, 0)
	inputBox.PackStart(a.sendBtn, false, false, 0)
	vbox.PackStart(inputBox, false, false, 0)

	// Recommendations
	recLabel, _ := gtk.LabelNew("Recommendations")
	recLabel.SetProperty("name", "QueueLabel")
	recLabel.SetHAlign(gtk.ALIGN_START)
	vbox.PackStart(recLabel, false, false, 0)

	a.recBox, _ = gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 8)
	vbox.PackStart(a.recBox, false, false, 0)

	// Action buttons
	a.actionBox, _ = gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 8)
	a.actionBox.SetNoShowAll(true)
	a.actionBox.Hide()
	a.approveBtn, _ = gtk.ButtonNewWithLabel("Approve")
	a.approveBtn.SetProperty("name", "BtnOpen")
	a.approveBtn.Connect("clicked", func() { a.onApprove() })
	a.modifyBtn, _ = gtk.ButtonNewWithLabel("Modify")
	a.modifyBtn.SetProperty("name", "btn-ghost")
	a.modifyBtn.Connect("clicked", func() { a.onModify() })
	a.denyBtn, _ = gtk.ButtonNewWithLabel("Deny")
	a.denyBtn.SetProperty("name", "btn-ghost")
	a.denyBtn.Connect("clicked", func() { a.onDeny() })
	a.actionBox.PackStart(a.approveBtn, false, false, 0)
	a.actionBox.PackStart(a.modifyBtn, false, false, 0)
	a.actionBox.PackStart(a.denyBtn, false, false, 0)
	vbox.PackStart(a.actionBox, false, false, 0)

	// Songlist view
	a.songlistScroll, _ = gtk.ScrolledWindowNew(nil, nil)
	a.songlistScroll.SetPolicy(gtk.POLICY_AUTOMATIC, gtk.POLICY_AUTOMATIC)
	a.songlistScroll.SetMinContentHeight(140)
	a.songlistScroll.SetProperty("name", "SonglistScroll")
	a.songlistScroll.SetNoShowAll(true)
	a.songlistScroll.Hide()
	a.songlistBox, _ = gtk.ListBoxNew()
	a.songlistScroll.Add(a.songlistBox)
	vbox.PackStart(a.songlistScroll, false, false, 0)

	// Status
	a.statusLabel, _ = gtk.LabelNew("")
	a.statusLabel.SetHAlign(gtk.ALIGN_START)
	a.statusLabel.SetProperty("name", "AlbumArtist")
	vbox.PackStart(a.statusLabel, false, false, 0)

	win.ShowAll()
}

func (a *App) applyCSS() {
	provider, _ := gtk.CssProviderNew()
	cssPath := AssetPath(filepath.Join("ui", "nozik.css"))
	if err := provider.LoadFromPath(cssPath); err != nil {
		log.Println("CSS load error:", err)
		return
	}
	screen, _ := gdk.ScreenGetDefault()
	if screen != nil {
		gtk.AddProviderForScreen(screen, provider, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)
	}
}

func (a *App) appendChat(sender, text string) {
	mark := ""
	switch sender {
	case "Nozik":
		mark = "\nNozik: "
	case "User":
		mark = "\nUser: "
	case "MCP":
		mark = "\nMCP: "
	default:
		mark = "\n" + sender + ": "
	}
	end := a.chatBuffer.GetEndIter()
	a.chatBuffer.Insert(end, mark+text+"\n")
	a.chatView.ScrollToIter(a.chatBuffer.GetEndIter(), 0, false, 0, 0)
}

func (a *App) setStatus(text string) {
	glib.IdleAdd(func() {
		a.statusLabel.SetText(text)
	})
}

func (a *App) onSend() {
	text, _ := a.entry.GetText()
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	a.entry.SetText("")
	a.appendChat("User", text)

	emotion := curation.DetectEmotion(text)
	mode := curation.ChooseMode(emotion, strings.Contains(strings.ToLower(text), "new") || strings.Contains(strings.ToLower(text), "discover"))
	ctx := context.Background()

	// First try local taste curation for a quick hit.
	localRecs, err := a.db.Discover(emotion, mode, 3)
	if err != nil {
		log.Println("db discover:", err)
	}

	go func() {
		strategy := a.strategyForSettings()
		var aiRecs []model.Recommendation
		if strategy != nil {
			history, _ := a.db.RecentPlays(10)
			req := ai.RecommendationRequest{
				Message:       text,
				Emotion:       emotion,
				RecentHistory: history,
				DiscoveryMode: mode,
				Limit:         3,
				SystemContext: fmt.Sprintf("Current emotion: %s. Discovery mode: %s. Use provider 'yt-music' if possible.", emotion, mode),
			}
			recs, err := strategy.RecommendTracks(ctx, req)
			if err != nil {
				a.setStatus(fmt.Sprintf("%s error: %v", strategy.Name(), err))
			} else {
				aiRecs = recs
			}
		}
		// Combine local taste + AI recommendations; dedupe by title+artist.
		seen := map[string]bool{}
		combined := []model.Recommendation{}
		for _, r := range append(localRecs, aiRecs...) {
			key := strings.ToLower(r.Title + "|" + r.Artist)
			if seen[key] || r.Title == "" {
				continue
			}
			seen[key] = true
			combined = append(combined, r)
		}
		if len(combined) == 0 {
			glib.IdleAdd(func() { a.appendChat("Nozik", "I couldn't find any recommendations right now.") })
			return
		}
		glib.IdleAdd(func() { a.showRecommendations(combined) })
	}()
}

func (a *App) strategyForSettings() ai.AIProviderStrategy {
	switch a.settings.Provider {
	case model.ProviderGemini:
		return ai.NewGeminiStrategy(a.settings.GeminiKey)
	case model.ProviderOpenAI:
		return ai.NewOpenAIStrategy(a.settings.OpenAIKey)
	case model.ProviderGrok:
		return ai.NewGrokStrategy(a.settings.GrokKey)
	case model.ProviderKimi:
		return ai.NewKimiStrategy(a.settings.KimiKey)
	case model.ProviderOther:
		return ai.NewLuauCustomStrategy(a.settings.Custom)
	default:
		return ai.NewGeminiStrategy(a.settings.GeminiKey)
	}
}

func (a *App) showRecommendations(recs []model.Recommendation) {
	// Clear existing recs.
	children := a.recBox.GetChildren()
	for children.Length() > 0 {
		a.recBox.Remove(children.NthData(0).(gtk.IWidget))
		children = a.recBox.GetChildren()
	}
	a.pendingRecs = recs
	for i, r := range recs {
		btn, _ := gtk.ButtonNewWithLabel(fmt.Sprintf("%d. %s - %s", i+1, r.Title, r.Artist))
		btn.SetTooltipText(r.Reason)
		btn.SetProperty("name", "BtnOpen")
		rec := r
		btn.Connect("clicked", func() { a.onRecommendationClicked(rec) })
		a.recBox.PackStart(btn, false, false, 0)
	}
	a.recBox.ShowAll()
	a.appendChat("Nozik", "Here are some recommendations. Click one to queue it.")
}

func (a *App) onRecommendationClicked(rec model.Recommendation) {
	a.selectedRec = &rec
	a.appendChat("Nozik", fmt.Sprintf("You clicked '%s' by %s.", rec.Title, rec.Artist))
	a.showSonglist([]model.QueueItem{{
		ProviderID:      rec.ProviderID,
		ProviderTrackID: rec.ProviderTrackID,
		Title:           rec.Title,
		Artist:          rec.Artist,
	}})
	a.actionBox.SetNoShowAll(false)
	a.actionBox.ShowAll()
}

func (a *App) showSonglist(items []model.QueueItem) {
	children := a.songlistBox.GetChildren()
	for children.Length() > 0 {
		a.songlistBox.Remove(children.NthData(0).(gtk.IWidget))
		children = a.songlistBox.GetChildren()
	}
	for _, it := range items {
		row, _ := gtk.ListBoxRowNew()
		lbl, _ := gtk.LabelNew(fmt.Sprintf("%s - %s", it.Title, it.Artist))
		lbl.SetHAlign(gtk.ALIGN_START)
		row.Add(lbl)
		a.songlistBox.Add(row)
	}
	a.songlistBox.ShowAll()
	a.songlistScroll.SetNoShowAll(false)
	a.songlistScroll.ShowAll()
}

func (a *App) onApprove() {
	if a.selectedRec == nil {
		return
	}
	a.appendChat("MCP", "Searching an Installation...")
	go func() {
		ctx := context.Background()
		if a.gozik == nil {
			client, err := gzk.Dial(config.GozikAddr())
			if err != nil {
				a.setStatus("Gozik not reachable: " + err.Error())
				glib.IdleAdd(func() { a.appendChat("Nozik", "Could not connect to Gozik. Make sure it is running.") })
				return
			}
			a.gozik = client
		}
		rec := a.selectedRec
		glib.IdleAdd(func() { a.appendChat("Nozik", "Approved. Calling a Gozik Player...") })
		providerID := rec.ProviderID
		if providerID == "" {
			providerID = "yt-music"
		}
		if rec.ProviderTrackID != "" {
			if err := a.gozik.AddProviderTrack(ctx, providerID, rec.ProviderTrackID); err != nil {
				a.setStatus("AddProviderTrack error: " + err.Error())
				return
			}
		} else {
			// Try provider search to resolve a track ID.
			tracks, err := a.gozik.SearchTracks(ctx, providerID, rec.Title+" "+rec.Artist, 1)
			if err != nil || len(tracks) == 0 {
				a.setStatus("Could not resolve track on Gozik")
				return
			}
			if err := a.gozik.AddProviderTrack(ctx, providerID, tracks[0].GetId()); err != nil {
				a.setStatus("AddProviderTrack error: " + err.Error())
				return
			}
		}
		_ = a.gozik.Play(ctx)
		// Record taste.
		emotion := curation.DetectEmotion("")
		_ = a.db.RecordPlay(model.PlayRecord{
			ProviderID:      providerID,
			ProviderTrackID: rec.ProviderTrackID,
			Title:           rec.Title,
			Artist:          rec.Artist,
			Album:           rec.Album,
			Emotion:         emotion,
			Weight:          1.0,
			PlayCount:       1,
		})
		glib.IdleAdd(func() { a.appendChat("Nozik", "Enjoy the music! I will wait for your next request.") })
	}()
}

func (a *App) onModify() {
	a.appendChat("Nozik", "Modify mode: please refine your request and send again.")
	a.actionBox.Hide()
}

func (a *App) onDeny() {
	a.selectedRec = nil
	a.actionBox.Hide()
	a.songlistScroll.Hide()
	a.appendChat("Nozik", "Got it. Let me know if you want something else.")
}

func (a *App) loadSettings() error {
	p := config.SettingsPath()
	if _, err := os.Stat(p); err != nil {
		return err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, &a.settings)
}

func (a *App) saveSettings() error {
	if err := config.EnsureAppDir(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(a.settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(config.SettingsPath(), b, 0644)
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
