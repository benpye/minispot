package main

import (
	"flag"
	"fmt"
	"github.com/gdamore/tcell"
	"github.com/librespot-org/librespot-golang/src/Spotify"
	"github.com/librespot-org/librespot-golang/src/librespot"
	"github.com/librespot-org/librespot-golang/src/librespot/core"
	"github.com/librespot-org/librespot-golang/src/librespot/utils"
	"github.com/rivo/tview"
	"io/ioutil"
	"strings"
	"sync"
	"time"
)

const (
	kDefaultDeviceName = "minispot"
)

func main() {
	app := tview.NewApplication()

	username := flag.String("username", "", "spotify username")
	blob := flag.String("blob", "blob.bin", "spotify auth blob")
	devicename := flag.String("devicename", kDefaultDeviceName, "name of device")
	flag.Parse()

	blobBytes, err := ioutil.ReadFile(*blob)

	if err != nil {
		fmt.Printf("Unable to read auth blob from %s: %s\n", *blob, err)
		return
	}

	session, err := librespot.LoginSaved(*username, blobBytes, *devicename)
	if err != nil {
		panic(err)
	}

	player, err := InitPlayer(session)
	if err != nil {
		panic(err)
	}
	defer player.Uninit()

	buildUI(app, session, player)

	if err := app.Run(); err != nil {
		panic(err)
	}
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second
	return fmt.Sprintf("%02d:%02d", m, s)
}

func buildUI(app *tview.Application, session *core.Session, player *Player) {
	sidebar := tview.NewTreeView()
	results := tview.NewTable()
	results.SetSelectable(true, false)
	results.SetBorders(false)

	statustext := tview.NewTextView()
	statustext.SetDynamicColors(true)
	statustext.SetBackgroundColor(tcell.ColorGray)
	statustext.SetTextColor(tcell.ColorWhite)
	progressindicator := tview.NewTextView()
	progressindicator.SetDynamicColors(true)
	progressindicator.SetBackgroundColor(tcell.ColorGray)
	progressindicator.SetTextColor(tcell.ColorWhite)
	progressindicatorstr := ""
	statusbar := tview.NewFlex().
		SetDirection(tview.FlexColumn).
		AddItem(statustext, 0, 1, false).
		AddItem(progressindicator, len("[00:00/00:00]"), 0, false)

	updateStatusText := func(player *Player) {
		playstate := "[red]s[white]"
		repeatstate := "[red]r[white]"
		shufflestate := "[red]s[white]"

		title := ""
		artist := ""

		seperator := ""

		if player.IsPlaying() {
			playstate = "[green::b]P[white::-]"

			if player.GetPauseState() {
				playstate = "[yellow::l]p[white::-]"
			}

			track := player.GetTrack()
			if track != nil {
				title = track.GetName()
				artist = track.GetArtist()[0].GetName()
				seperator = " [::d]-[::-] "
			}
		}

		app.QueueUpdateDraw(func() {
			statustext.SetText(fmt.Sprintf("[::d][[::-]%s%s%s[::d]][::-] %s%s%s", repeatstate, shufflestate, playstate, title, seperator, artist))
		})
	}

	updateProgressText := func(player *Player) {
		progressstr := "00:00"
		lengthstr := "00:00"

		track := player.GetTrack()

		if player.IsPlaying() && track != nil {
			progress := time.Duration(player.GetPlayProgress()) * time.Millisecond
			length := time.Duration(track.GetDuration()) * time.Millisecond

			progressstr = formatDuration(progress)
			lengthstr = formatDuration(length)
		}

		str := fmt.Sprintf("[::d][[::-]%s[::d]/[::-]%s[::d]][::-]", progressstr, lengthstr)

		if progressindicatorstr != str {
			progressindicatorstr = str
			app.QueueUpdateDraw(func() {
				progressindicator.SetText(str)
			})
		}
	}

	updateStatusText(player)
	updateProgressText(player)

	commandbar := tview.NewInputField()

	nowplayingnode := tview.NewTreeNode("Now Playing").SetSelectable(true)
	searchresultnode := tview.NewTreeNode("Search Results").SetSelectable(true)
	playlistsnode := tview.NewTreeNode("Playlists").SetSelectable(true)

	sidebar.SetRoot(
		tview.NewTreeNode("").
			AddChild(nowplayingnode).
			AddChild(searchresultnode).
			AddChild(playlistsnode))

	player.SetOnTrackStartedCallback(func(track *Spotify.Track) {
		updateProgressText(player)
		updateStatusText(player)
	})

	player.SetOnTrackFinishedCallback(func() {
		updateProgressText(player)
		updateStatusText(player)
	})

	player.SetOnTrackPausedCallback(func(track *Spotify.Track, pause bool) {
		updateStatusText(player)
	})

	player.SetOnTrackProgressCallback(func() {
		updateProgressText(player)
	})

	var curList *sync.Map
	var playlistId string

	playlist, _ := session.Mercury().GetRootPlaylist(session.Username())
	for _, item := range playlist.Contents.Items {
		id := strings.TrimPrefix(item.GetUri(), "spotify:")
		id = strings.Replace(id, ":", "/", -1)
		list, _ := session.Mercury().GetPlaylist(id)
		playlistsnode.AddChild(tview.NewTreeNode(list.Attributes.GetName()).SetSelectedFunc(func() {
			playlistId = id
			go func() {
				curList = &sync.Map{}

				results.Clear()
				results.SetCell(0, 0, tview.NewTableCell("Track").SetMaxWidth(1).SetExpansion(1).SetSelectable(false).SetBackgroundColor(tcell.ColorGray).SetTextColor(tcell.ColorWhite))
				results.SetCell(0, 1, tview.NewTableCell("Album").SetMaxWidth(1).SetExpansion(1).SetSelectable(false).SetBackgroundColor(tcell.ColorGray).SetTextColor(tcell.ColorWhite))
				results.SetCell(0, 2, tview.NewTableCell("Artist").SetMaxWidth(1).SetExpansion(1).SetSelectable(false).SetBackgroundColor(tcell.ColorGray).SetTextColor(tcell.ColorWhite))
				results.SetFixed(1, 0)

				items := list.Contents.Items
				row := 1
				for i := 0; i < len(items); i++ {
					id := strings.TrimPrefix(items[i].GetUri(), "spotify:track:")
					track, err := session.Mercury().GetTrack(utils.Base62ToHex(id))
					if err != nil || track.GetGid() == nil {
						continue
					}
					results.SetCell(row, 0, tview.NewTableCell(track.GetName()).SetMaxWidth(1).SetExpansion(1))
					results.SetCell(row, 1, tview.NewTableCell(track.GetAlbum().GetName()).SetMaxWidth(1).SetExpansion(1))
					results.SetCell(row, 2, tview.NewTableCell(track.GetArtist()[0].GetName()).SetMaxWidth(1).SetExpansion(1))
					curList.Store(row, id)
					row++
					//results.AddItem(fmt.Sprintf("%s - %s", track.GetName(), track.GetAlbum().GetName()), id, 0, nil)
				}
			}()
		}))
	}

	results.SetSelectedFunc(func(row int, column int) {
		if curList == nil {
			return
		}

		list, err := InitSpotifyPlaylist(session, playlistId)
		if err != nil {
			return
		}

		go player.PlayPlaylist(list, row-1)
	})

	sidebar.SetTopLevel(1)
	sidebar.SetCurrentNode(nowplayingnode)

	flex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(tview.NewFlex().SetDirection(tview.FlexColumn).
			AddItem(sidebar, 0, 1, false).
			AddItem(NewDivider(), 1, 0, false).
			AddItem(results, 0, 3, false), 0, 1, false).
		AddItem(statusbar, 1, 1, false).
		AddItem(commandbar, 1, 1, false)

	commandbar.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			str := strings.TrimSpace(commandbar.GetText())
			switch str[0] {
			case '/':
				// Do search
				//results.AddItem("Search", "", 0, nil)
			case ':':
				// Do command
				//results.AddItem("Command", "", 0, nil)
			default:
				// Fail - wah
				//results.AddItem("Unknown", "", 0, nil)
			}

			commandbar.SetText("")
			app.SetFocus(flex)
		} else {
			commandbar.SetText("")
		}
	})

	app.SetRoot(flex, true)

	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyESC {
			app.SetFocus(flex)
		} else if app.GetFocus() == flex && event.Key() == tcell.KeyRune {
			// We are in "command" mode
			if event.Rune() == ' ' {
				player.SetPauseState(!player.GetPauseState())
			} else if event.Rune() == 's' {
				app.SetFocus(sidebar)
			} else if event.Rune() == 'l' {
				app.SetFocus(results)
			} else if event.Rune() == '/' || event.Rune() == ':' {
				app.SetFocus(commandbar)
				commandbar.SetText(string(event.Rune()))
			}
		}

		return event
	})
}
