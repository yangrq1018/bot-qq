package modules

import "time"

type endpoint int

const (
	createUser endpoint = iota
)

// UserDto represents an EMBY user
type UserDto struct {
	Name                      string    `json:"Name"`
	ServerId                  string    `json:"ServerId"`
	Prefix                    string    `json:"Prefix"`
	DateCreated               time.Time `json:"DateCreated"`
	Id                        string    `json:"Id"`
	HasPassword               bool      `json:"HasPassword"`
	HasConfiguredPassword     bool      `json:"HasConfiguredPassword"`
	HasConfiguredEasyPassword bool      `json:"HasConfiguredEasyPassword"`
	Configuration             struct {
		PlayDefaultAudioTrack      bool          `json:"PlayDefaultAudioTrack"`
		DisplayMissingEpisodes     bool          `json:"DisplayMissingEpisodes"`
		SubtitleMode               string        `json:"SubtitleMode"`
		EnableLocalPassword        bool          `json:"EnableLocalPassword"`
		OrderedViews               []interface{} `json:"OrderedViews"`
		LatestItemsExcludes        []interface{} `json:"LatestItemsExcludes"`
		MyMediaExcludes            []interface{} `json:"MyMediaExcludes"`
		HidePlayedInLatest         bool          `json:"HidePlayedInLatest"`
		RememberAudioSelections    bool          `json:"RememberAudioSelections"`
		RememberSubtitleSelections bool          `json:"RememberSubtitleSelections"`
		EnableNextEpisodeAutoPlay  bool          `json:"EnableNextEpisodeAutoPlay"`
		ResumeRewindSeconds        int           `json:"ResumeRewindSeconds"`
		IntroSkipMode              string        `json:"IntroSkipMode"`
	} `json:"Configuration"`
	Policy struct {
		IsAdministrator                  bool          `json:"IsAdministrator"`
		IsHidden                         bool          `json:"IsHidden"`
		IsHiddenRemotely                 bool          `json:"IsHiddenRemotely"`
		IsHiddenFromUnusedDevices        bool          `json:"IsHiddenFromUnusedDevices"`
		IsDisabled                       bool          `json:"IsDisabled"`
		BlockedTags                      []interface{} `json:"BlockedTags"`
		IsTagBlockingModeInclusive       bool          `json:"IsTagBlockingModeInclusive"`
		IncludeTags                      []interface{} `json:"IncludeTags"`
		EnableUserPreferenceAccess       bool          `json:"EnableUserPreferenceAccess"`
		AccessSchedules                  []interface{} `json:"AccessSchedules"`
		BlockUnratedItems                []interface{} `json:"BlockUnratedItems"`
		EnableRemoteControlOfOtherUsers  bool          `json:"EnableRemoteControlOfOtherUsers"`
		EnableSharedDeviceControl        bool          `json:"EnableSharedDeviceControl"`
		EnableRemoteAccess               bool          `json:"EnableRemoteAccess"`
		EnableLiveTvManagement           bool          `json:"EnableLiveTvManagement"`
		EnableLiveTvAccess               bool          `json:"EnableLiveTvAccess"`
		EnableMediaPlayback              bool          `json:"EnableMediaPlayback"`
		EnableAudioPlaybackTranscoding   bool          `json:"EnableAudioPlaybackTranscoding"`
		EnableVideoPlaybackTranscoding   bool          `json:"EnableVideoPlaybackTranscoding"`
		EnablePlaybackRemuxing           bool          `json:"EnablePlaybackRemuxing"`
		EnableContentDeletion            bool          `json:"EnableContentDeletion"`
		EnableContentDeletionFromFolders []interface{} `json:"EnableContentDeletionFromFolders"`
		EnableContentDownloading         bool          `json:"EnableContentDownloading"`
		EnableSubtitleDownloading        bool          `json:"EnableSubtitleDownloading"`
		EnableSubtitleManagement         bool          `json:"EnableSubtitleManagement"`
		EnableSyncTranscoding            bool          `json:"EnableSyncTranscoding"`
		EnableMediaConversion            bool          `json:"EnableMediaConversion"`
		EnabledChannels                  []interface{} `json:"EnabledChannels"`
		EnableAllChannels                bool          `json:"EnableAllChannels"`
		EnabledFolders                   []interface{} `json:"EnabledFolders"`
		EnableAllFolders                 bool          `json:"EnableAllFolders"`
		InvalidLoginAttemptCount         int           `json:"InvalidLoginAttemptCount"`
		EnablePublicSharing              bool          `json:"EnablePublicSharing"`
		RemoteClientBitrateLimit         int           `json:"RemoteClientBitrateLimit"`
		ExcludedSubFolders               []interface{} `json:"ExcludedSubFolders"`
		SimultaneousStreamLimit          int           `json:"SimultaneousStreamLimit"`
		EnabledDevices                   []interface{} `json:"EnabledDevices"`
		EnableAllDevices                 bool          `json:"EnableAllDevices"`
	} `json:"Policy"`
}
