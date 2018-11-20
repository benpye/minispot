package main

import (
	"github.com/librespot-org/librespot-golang/src/Spotify"
	"github.com/librespot-org/librespot-golang/src/librespot/core"
)

type Playlist interface {
	Name() string
	Length() int
	GetTrackAt(int) string
}

type SpotifyPlaylist struct {
	// Underlying Spotify playlist
	playlist *Spotify.SelectedListContent
}

func InitSpotifyPlaylist(session *core.Session, id string) (sp *SpotifyPlaylist, err error) {
	list, err := session.Mercury().GetPlaylist(id)
	if err != nil {
		return
	}

	sp = &SpotifyPlaylist{list}
	return
}

func (sp *SpotifyPlaylist) Name() string {
	return sp.playlist.Attributes.GetName()
}

func (sp *SpotifyPlaylist) Length() int {
	return len(sp.playlist.Contents.Items)
}

func (sp *SpotifyPlaylist) GetTrackAt(idx int) string {
	return sp.playlist.Contents.Items[idx].GetUri()
}
