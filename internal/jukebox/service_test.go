package jukebox

import "testing"

func TestGenreMatchStrength(t *testing.T) {
	if got := genreMatchStrength("Viking Rock", "Viking Rock"); got != 1 {
		t.Fatalf("expected exact match strength 1, got %v", got)
	}
	if got := genreMatchStrength("Viking Rock", "Viking Metal"); got <= 0 {
		t.Fatalf("expected partial genre match, got %v", got)
	}
	if got := genreMatchStrength("Industrial Electronic", "Melodic Folk"); got != 0 {
		t.Fatalf("expected no genre match, got %v", got)
	}
}

func TestNormalizeModeMapsLegacySeedsToRadio(t *testing.T) {
	if got := normalizeMode("radio"); got != "radio" {
		t.Fatalf("expected radio, got %q", got)
	}
	for _, legacy := range []string{"genre", "creator", "album"} {
		if got := normalizeMode(legacy); got != "radio" {
			t.Fatalf("expected %q to map to radio, got %q", legacy, got)
		}
	}
}

func TestFilterCandidatesForModeGenre(t *testing.T) {
	candidates := []candidateTrack{
		{ID: "1", Genre: "Viking Rock"},
		{ID: "2", Genre: "Viking Metal"},
		{ID: "3", Genre: "Dark Ambient"},
	}
	filtered := filterCandidatesForMode(candidates, sessionState{Mode: "genre", SeedGenre: "Viking Rock"}, tasteProfile{})
	if len(filtered) != 2 {
		t.Fatalf("expected 2 genre candidates, got %d", len(filtered))
	}
}

func TestFilterCandidatesForModeCreator(t *testing.T) {
	candidates := make([]candidateTrack, 0, 30)
	for i := 0; i < 12; i++ {
		candidates = append(candidates, candidateTrack{ID: string(rune('a' + i)), OwnerSub: "creator-a", Genre: "Viking Rock"})
	}
	for i := 0; i < 12; i++ {
		candidates = append(candidates, candidateTrack{ID: string(rune('m' + i)), OwnerSub: "creator-b", Genre: "Viking Metal"})
	}
	candidates = append(candidates,
		candidateTrack{ID: "x1", OwnerSub: "creator-c", Genre: "Dark Ambient"},
		candidateTrack{ID: "x2", OwnerSub: "creator-d", Genre: "Synthwave"},
	)
	filtered := filterCandidatesForMode(candidates, sessionState{Mode: "creator", SeedCreatorSub: "creator-a"}, tasteProfile{})
	if len(filtered) != 24 {
		t.Fatalf("expected 24 creator candidates, got %d", len(filtered))
	}
}

func TestFilterCandidatesForModeCreatorDoesNotFallBackToGlobalPool(t *testing.T) {
	candidates := []candidateTrack{
		{ID: "1", OwnerSub: "creator-a", Genre: "Viking Rock"},
		{ID: "2", OwnerSub: "creator-a", Genre: "Viking Rock"},
		{ID: "3", OwnerSub: "creator-b", Genre: "Dark Ambient"},
	}
	filtered := filterCandidatesForMode(candidates, sessionState{Mode: "creator", SeedCreatorSub: "creator-a"}, tasteProfile{})
	if len(filtered) != 2 {
		t.Fatalf("expected creator mode to keep the focused subset, got %d", len(filtered))
	}
}

func TestFilterCandidatesForModeAlbum(t *testing.T) {
	candidates := make([]candidateTrack, 0, 30)
	for i := 0; i < 12; i++ {
		candidates = append(candidates, candidateTrack{ID: string(rune('a' + i)), AlbumID: 5, Artist: "AsaTyr", Genre: "Viking Rock"})
	}
	for i := 0; i < 12; i++ {
		candidates = append(candidates, candidateTrack{ID: string(rune('m' + i)), AlbumID: 7, Artist: "AsaTyr", Genre: "Viking Metal"})
	}
	for i := 0; i < 6; i++ {
		candidates = append(candidates, candidateTrack{ID: string(rune('z' + i)), AlbumID: int64(20 + i), Artist: "Other", Genre: "Dark Ambient"})
	}
	filtered := filterCandidatesForMode(candidates, sessionState{Mode: "album", SeedAlbumID: 5}, tasteProfile{})
	if len(filtered) != 24 {
		t.Fatalf("expected 24 album candidates, got %d", len(filtered))
	}
}

func TestFilterCandidatesForModeAlbumDoesNotFallBackToGlobalPool(t *testing.T) {
	candidates := []candidateTrack{
		{ID: "1", AlbumID: 5, Artist: "AsaTyr", Genre: "Viking Rock"},
		{ID: "2", AlbumID: 5, Artist: "AsaTyr", Genre: "Viking Rock"},
		{ID: "3", AlbumID: 9, Artist: "Other", Genre: "Dark Ambient"},
	}
	filtered := filterCandidatesForMode(candidates, sessionState{Mode: "album", SeedAlbumID: 5}, tasteProfile{})
	if len(filtered) != 2 {
		t.Fatalf("expected album mode to keep the focused subset, got %d", len(filtered))
	}
}

func TestFilterCandidatesForModeTryMePrefersLowPlayPool(t *testing.T) {
	candidates := []candidateTrack{
		{ID: "1", Plays: 2, RecentScore: 0},
		{ID: "2", Plays: 3, RecentScore: 1},
		{ID: "3", Plays: 4, RecentScore: 2},
		{ID: "4", Plays: 100, RecentScore: 10},
	}
	filtered := filterCandidatesForMode(candidates, sessionState{Mode: "try_me"}, tasteProfile{})
	if len(filtered) != 3 {
		t.Fatalf("expected low-play pool to narrow results, got %d", len(filtered))
	}
	for _, item := range filtered {
		if item.ID == "4" {
			t.Fatalf("expected the high-play track to be excluded")
		}
	}
}

func TestDiversifyTracksCapsAlbumDominance(t *testing.T) {
	scored := []scoredTrack{
		{candidateTrack: candidateTrack{ID: "a1", AlbumID: 29, Genre: "Cabaret"}, Score: 200},
		{candidateTrack: candidateTrack{ID: "a2", AlbumID: 29, Genre: "Cabaret"}, Score: 199},
		{candidateTrack: candidateTrack{ID: "a3", AlbumID: 29, Genre: "Cabaret"}, Score: 198},
		{candidateTrack: candidateTrack{ID: "a4", AlbumID: 29, Genre: "Cabaret"}, Score: 197},
		{candidateTrack: candidateTrack{ID: "b1", AlbumID: 117, Genre: "Industrial Electronic"}, Score: 180},
		{candidateTrack: candidateTrack{ID: "b2", AlbumID: 117, Genre: "Industrial Electronic"}, Score: 179},
		{candidateTrack: candidateTrack{ID: "c1", AlbumID: 251, Genre: "Horror Classical"}, Score: 170},
		{candidateTrack: candidateTrack{ID: "d1", AlbumID: 300, Genre: "Epic Metal"}, Score: 169},
	}
	picks := diversifyTracks(scored, 6, "creator")
	counts := map[int64]int{}
	for _, pick := range picks {
		counts[pick.AlbumID]++
	}
	if counts[29] > 2 {
		t.Fatalf("expected album 29 to be capped at 2 picks, got %d", counts[29])
	}
}
