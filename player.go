package main

import (
	"errors"
	"fmt"
	"github.com/gen2brain/malgo"
	"github.com/jfreymuth/oggvorbis"
	"github.com/librespot-org/librespot-golang/src/Spotify"
	"github.com/librespot-org/librespot-golang/src/librespot/core"
	"github.com/librespot-org/librespot-golang/src/librespot/player"
	"github.com/librespot-org/librespot-golang/src/librespot/utils"
	"io"
	"reflect"
	"strings"
	"sync"
	"unsafe"
)

const (
	kSamplesPerChannel = 2048
	kSampleRate        = 44100
	kChannels          = 2
)

type OnTrackStartedFunc func(*Spotify.Track)
type OnTrackPausedFunc func(*Spotify.Track, bool)
type OnTrackFinishedFunc func()
type OnTrackProgressFunc func()

type Player struct {
	// Spotify session
	session *core.Session

	// Audio device
	ctx    *malgo.AllocatedContext
	device *malgo.Device

	// Ogg Vorbis decoder
	audioFile   *player.AudioFile
	reader      *oggvorbis.Reader
	readerMutex *sync.Mutex

	// Callbacks
	onTrackStarted  OnTrackStartedFunc
	onTrackPaused   OnTrackPausedFunc
	onTrackFinished OnTrackFinishedFunc
	onTrackProgress OnTrackProgressFunc

	// Current playback state
	// XXX: Might need a mutex on playback state?
	//playlist *Spotify.SelectedListContent
	playlist    Playlist
	playlistIdx int

	track   *Spotify.Track
	playing bool
	paused  bool
}

func InitPlayer(session *core.Session) (player *Player, err error) {
	player = &Player{}
	player.readerMutex = &sync.Mutex{}
	player.session = session
	player.playing = false
	player.paused = false

	// TODO: Allow backend selection?
	player.ctx, err = malgo.InitContext(nil, malgo.ContextConfig{}, func(message string) {})
	if err != nil {
		player.Uninit()
		return
	}

	deviceConfig := malgo.DefaultDeviceConfig()
	deviceConfig.Format = malgo.FormatF32
	deviceConfig.Channels = kChannels
	deviceConfig.SampleRate = kSampleRate
	deviceConfig.BufferSizeInFrames = kSamplesPerChannel
	deviceConfig.PerformanceProfile = malgo.Conservative

	deviceCallbacks := malgo.DeviceCallbacks{
		Send: player.sendCallback(),
	}

	player.device, err = malgo.InitDevice(player.ctx.Context, malgo.Playback, nil, deviceConfig, deviceCallbacks)
	if err != nil {
		player.Uninit()
		return
	}

	if player.device.IsStarted() {
		player.device.Stop()
	}

	return
}

func (player *Player) Uninit() {
	if player.ctx != nil {
		player.ctx.Free()
	}

	if player.device != nil {
		player.device.Uninit()
	}
}

func getPreferredAudioFile(track *Spotify.Track) (selectedFile *Spotify.AudioFile) {
	for _, file := range track.GetFile() {
		// Take highest quality available ogg vorbis file
		if (file.GetFormat() == Spotify.AudioFile_OGG_VORBIS_96 && selectedFile == nil) ||
			(file.GetFormat() == Spotify.AudioFile_OGG_VORBIS_160 && (selectedFile == nil || selectedFile.GetFormat() == Spotify.AudioFile_OGG_VORBIS_96)) ||
			(file.GetFormat() == Spotify.AudioFile_OGG_VORBIS_320) {
			selectedFile = file
		}
	}

	return
}

func (player *Player) PlayPlaylist(playlist Playlist, idx int) error {
	if idx >= playlist.Length() {
		return errors.New("idx outside playlist bounds")
	}

	err := player.playPlaylistTrack(playlist, idx)
	if err != nil {
		return err
	}

	player.playlist = playlist
	player.playlistIdx = idx

	return nil
}

type localTrackError struct{}

func (*localTrackError) Error() string {
	return "Local tracks not supported"
}

func (player *Player) playPlaylistTrack(list Playlist, idx int) error {
	trackURI := list.GetTrackAt(idx)
	if !strings.HasPrefix(trackURI, "spotify:track:") {
		return &localTrackError{}
	}

	err := player.Play(strings.TrimPrefix(trackURI, "spotify:track:"))
	if err != nil {
		return err
	}

	return nil
}

func (player *Player) PlayNext() (ok bool, err error) {
	ok = false

	if player.playlist == nil {
		err = errors.New("No playlist set")
		return
	}

	newIdx := player.playlistIdx + 1
	if newIdx >= player.playlist.Length() {
		return
	}

	err = player.playPlaylistTrack(player.playlist, newIdx)
	if err != nil {
		return
	}

	player.playlistIdx = newIdx
	ok = true
	return
}

func (player *Player) Play(id string) error {
	track, err := player.session.Mercury().GetTrack(utils.Base62ToHex(id))
	if err != nil {
		return err
	}

	selectedFile := getPreferredAudioFile(track)
	if selectedFile == nil {
		for _, altTrack := range track.GetAlternative() {
			selectedFile = getPreferredAudioFile(altTrack)

			if selectedFile != nil {
				break
			}
		}
	}

	if selectedFile == nil {
		return errors.New(fmt.Sprintf("Could not find track for id: %s", id))
	}

	audioFile, err := player.session.Player().LoadTrack(selectedFile, track.GetGid())
	if err != nil {
		return err
	}

	dec, err := oggvorbis.NewReader(audioFile)
	if err != nil {
		return err
	}

	player.readerMutex.Lock()
	if player.audioFile != nil {
		player.audioFile.Cancel()
	}
	player.audioFile = audioFile
	player.reader = dec
	player.readerMutex.Unlock()

	if !player.device.IsStarted() {
		player.device.Start()
	}

	player.paused = false
	player.playing = true
	player.track = track

	if player.onTrackStarted != nil {
		player.onTrackStarted(player.track)
	}

	return nil
}

func (player *Player) SetPauseState(pause bool) {
	if !player.playing || pause == player.paused {
		return
	}

	if pause {
		player.device.Stop()
	} else {
		player.device.Start()
	}

	player.paused = pause

	if player.onTrackPaused != nil {
		player.onTrackPaused(player.track, pause)
	}
}

func (player *Player) GetPauseState() bool {
	return player.paused
}

func (player *Player) GetTrack() *Spotify.Track {
	return player.track
}

func (player *Player) IsPlaying() bool {
	return player.playing
}

func (player *Player) GetPlayProgress() int {
	reader := player.reader
	if reader == nil {
		return 0
	}

	return int((1000 * reader.Position()) / kSampleRate)
}

func (player *Player) SetOnTrackStartedCallback(cb OnTrackStartedFunc) {
	player.onTrackStarted = cb
}

func (player *Player) SetOnTrackPausedCallback(cb OnTrackPausedFunc) {
	player.onTrackPaused = cb
}

func (player *Player) SetOnTrackFinishedCallback(cb OnTrackFinishedFunc) {
	player.onTrackFinished = cb
}

func (player *Player) SetOnTrackProgressCallback(cb OnTrackProgressFunc) {
	player.onTrackProgress = cb
}

func (player *Player) trackFinished() {
	if player.onTrackFinished != nil {
		player.onTrackFinished()
	}

	ok, _ := player.PlayNext()
	if ok == true {
		return
	}

	// Stop the audio output device - power saving
	if player.device.IsStarted() {
		player.device.Stop()
	}
}

func (player *Player) sendCallback() func(frameCount uint32, samples []byte) uint32 {
	return func(frameCount uint32, output []byte) uint32 {
		// We don't want to call the finished callback multiple times
		if !player.playing {
			return 0
		}

		/*
		 * XXX: BEWARE DRAGONS BELOW
		 *      THIS IS A HACK TO AVOID COPYING SAMPLES
		 */

		player.readerMutex.Lock()
		defer player.readerMutex.Unlock()

		if player.reader == nil {
			return 0
		}

		header := *(*reflect.SliceHeader)(unsafe.Pointer(&output))

		header.Len /= 4
		header.Cap /= 4

		data := *(*[]float32)(unsafe.Pointer(&header))

		// It seems most APIs need us to fill the buffer
		n := 0
		for uint32(n) < frameCount*uint32(kChannels) {
			nt, err := player.reader.Read(data[n:])
			n += nt
			if err == io.EOF {
				player.playing = false
				go player.trackFinished()
				break
			}
		}

		go player.onTrackProgress()

		return uint32(n) / uint32(kChannels)
	}
}
