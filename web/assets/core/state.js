(function(ns) {
    const state = {
      token: localStorage.getItem('hex_token') || '',
      refreshToken: localStorage.getItem('hex_refresh_token') || '',
      tokenExpUnix: Number(localStorage.getItem('hex_token_exp_unix') || '0'),
      inviteToken: '',
      invitePageMode: false,
      me: null,
      tracks: [],
      albums: [],
      filteredTracks: [],
      filteredAlbums: [],
      selectedAlbum: null,
      selectedAlbumComments: [],
      tracksAlbumContextID: 0,
      selectedUserSub: '',
      selectedUserHandle: '',
      currentProfileTab: 'identity',
      selectedPublicUserProfile: null,
      selectedPublicUserUploads: { albums: [], tracks: [] },
      selectedPublicUserComments: [],
      creatorCenterTab: 'overview',
      profileDirty: false,
      profileSaveState: '',
      profileSnapshot: '',
      selectedAdminTrackId: '',
      selectedDetailTrackId: '',
      selectedDetailTrackData: null,
      selectedManageAlbumId: 0,
      manageTracks: [],
      manageAlbums: [],
      adminUsers: [],
      adminInvites: [],
      publicSettings: { registration_enabled: false },
      adminSystemOverview: null,
      discovery: null,
      discoveryTab: 'top',
      jukebox: {
        mode: 'for_you',
        session_id: '',
        queue: [],
        current_position: 0,
        seed_genre: '',
        seed_creator_sub: '',
        seed_album_id: 0
      },
      myProfile: null,
      creatorStats: null,
      creatorHighscore: [],
      notifications: [],
      notificationsUnread: 0,
      notificationPollTimer: null,
      favoritesTab: 'albums',
      favorites: { tracks: [], albums: [], playlists: [] },
      playlists: [],
      selectedPlaylistId: 0,
      selectedPlaylistTracks: [],
      playlistPickerTrackIDs: [],
      playlistDockOpen: localStorage.getItem('hex_playlist_dock_open') === '1',
      manualQueueOpen: localStorage.getItem('hex_manual_queue_open') === '1',
      albumCoverURLs: {},
      currentView: 'discovery',
      albumGenreFilter: 'all',
      trackGenreFilter: 'all',
      jukeboxPopoutMode: false,
      activeEngine: 'main',
      mainPlayer: {
        queue: [],
        queueIndex: -1,
        nowPlayingTrackId: '',
        shuffle: localStorage.getItem('hex_main_shuffle') === '1',
        repeatMode: localStorage.getItem('hex_main_repeat_mode') || 'off',
        currentLyrics: { track_id: '', plain: '', srt: '', cues: [] }
      },
      jukeboxPlayer: {
        queue: [],
        queueIndex: -1,
        nowPlayingTrackId: '',
        currentLyrics: { track_id: '', plain: '', srt: '', cues: [] },
        activeAudioId: 'audioJukeboxA',
        standbyAudioId: 'audioJukeboxB',
        currentSourceContext: '',
        crossfadeSeconds: 5,
        crossfadeTimer: null,
        crossfading: false,
        crossfadeTrackId: '',
        crossfadeTargetIndex: -1
      },
      queue: [],
      queueIndex: -1,
      nowPlayingTrackId: '',
      currentTrackMeta: null,
      currentCoverURL: '',
      jukeboxCurrentCoverURL: '',
      audioFX: null,
      currentLyrics: { track_id: '', plain: '', srt: '', cues: [] },
      popoutMode: false,
      popoutWindow: null,
      playerBridge: null,
      jukeboxBridge: null,
      popoutSnapshot: null,
      jukeboxPopoutSnapshot: null,
      popoutLastLyricIndex: -1,
      lastBridgeStateAt: 0,
      lastFullBridgeStateAt: 0,
      popoutSyncTimer: null,
      jukeboxPopoutSyncTimer: null,
      bridgeTickTimer: null,
      popVizMode: localStorage.getItem('hex_pop_viz_mode') || 'bars',
      popLyricAutoScroll: localStorage.getItem('hex_pop_lyric_autoscroll') !== '0',
      popLyricFontPx: Number(localStorage.getItem('hex_pop_lyric_font_px') || '13'),
      popQueueFilter: '',
      popQueueFollowCurrent: localStorage.getItem('hex_pop_queue_follow') !== '0',
      popVizPrevBins: [],
      listeningSession: null,
      streamSignCache: {}
    };
    const ICON_PLAY = '\u25b6';
    const ICON_PAUSE = '\u23f8';
    const ICON_MUTE = '\ud83d\udd07';
    const ICON_UNMUTE = '\ud83d\udd0a';
    const ICON_DETAIL = '\u2139';
    const ICON_PLAYLIST = '\u2261';
    const PLAYER_BRIDGE_CHANNEL = 'hexsonic-player-bridge-v1';
    const JUKEBOX_BRIDGE_CHANNEL = 'hexsonic-jukebox-bridge-v1';
    const PLAYER_TICK_MS = 34;

    const $ = (id) => document.getElementById(id);
    const escapeHtml = (value) => String(value ?? '')
      .replaceAll('&', '&amp;')
      .replaceAll('<', '&lt;')
      .replaceAll('>', '&gt;')
      .replaceAll('"', '&quot;')
      .replaceAll("'", '&#39;');
    const SELECTED_ALBUM_KEY = 'hex_selected_album_id';
  Object.assign(ns, {
    state,
    ICON_PLAY,
    ICON_PAUSE,
    ICON_MUTE,
    ICON_UNMUTE,
    ICON_DETAIL,
    ICON_PLAYLIST,
    PLAYER_BRIDGE_CHANNEL,
    JUKEBOX_BRIDGE_CHANNEL,
    PLAYER_TICK_MS,
    SELECTED_ALBUM_KEY
  });
})(window.HexSonic = window.HexSonic || {});
