package Youtube

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"

	"github.com/dop251/goja"
)

const (
	Video2160p  = "2160p"
	Video1440p  = "1440p"
	Video1080p  = "1080p"
	Video720p   = "720p"
	Video480p   = "480p"
	Video360p   = "360p"
	Video240p   = "240p"
	Video144p   = "144p"
	AudioMedium = "AUDIO_QUALITY_MEDIUM"
	AudioLow    = "AUDIO_QUALITY_LOW"
)

type YTVids struct {
	RBody io.ReadCloser
	Body  []byte
	Index int
}

type YTRequest struct {
	ContentLength string
	URL           string
	VideoID       string
	Sig           bool
	Config        Youtube
}

type Youtube struct {
	ID           string
	FullURL      string
	Title        string
	Length       string
	Index        string
	Info         string
	Continuation string
	MS           int
}

type YT struct {
	StreamingData StreamingData `json:"streamingData"`
	VideoDetails  Details       `json:"videoDetails"`
}
type Details struct {
	VideoID          string   `json:"videoId"`
	Title            string   `json:"title"`
	LengthSeconds    string   `json:"lengthSeconds"`
	Keywords         []string `json:"keywords"`
	ChannelID        string   `json:"channelId"`
	IsOwnerViewing   bool     `json:"isOwnerViewing"`
	ShortDescription string   `json:"shortDescription"`
	IsCrawlable      bool     `json:"isCrawlable"`
	Thumbnail        struct {
		Thumbnails []struct {
			URL    string `json:"url"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
		} `json:"thumbnails"`
	} `json:"thumbnail"`
	AllowRatings      bool   `json:"allowRatings"`
	ViewCount         string `json:"viewCount"`
	Author            string `json:"author"`
	IsPrivate         bool   `json:"isPrivate"`
	IsUnpluggedCorpus bool   `json:"isUnpluggedCorpus"`
	IsLiveContent     bool   `json:"isLiveContent"`
}
type Formats struct {
	Itag             int    `json:"itag"`
	URL              string `json:"url"`
	MimeType         string `json:"mimeType"`
	Bitrate          int    `json:"bitrate"`
	Width            int    `json:"width"`
	Height           int    `json:"height"`
	LastModified     string `json:"lastModified"`
	Quality          string `json:"quality"`
	Fps              int    `json:"fps"`
	QualityLabel     string `json:"qualityLabel"`
	ProjectionType   string `json:"projectionType"`
	AudioQuality     string `json:"audioQuality"`
	ApproxDurationMs string `json:"approxDurationMs"`
	AudioSampleRate  string `json:"audioSampleRate"`
	AudioChannels    int    `json:"audioChannels"`
	Sig              string `json:"signatureCipher"`
}
type InitRange struct {
	Start string `json:"start"`
	End   string `json:"end"`
}
type IndexRange struct {
	Start string `json:"start"`
	End   string `json:"end"`
}
type ColorInfo struct {
	Primaries               string `json:"primaries"`
	TransferCharacteristics string `json:"transferCharacteristics"`
	MatrixCoefficients      string `json:"matrixCoefficients"`
}
type AdaptiveFormats struct {
	Itag             int        `json:"itag"`
	URL              string     `json:"url"`
	MimeType         string     `json:"mimeType"`
	Bitrate          int        `json:"bitrate"`
	Width            int        `json:"width,omitempty"`
	Height           int        `json:"height,omitempty"`
	InitRange        InitRange  `json:"initRange"`
	IndexRange       IndexRange `json:"indexRange"`
	LastModified     string     `json:"lastModified"`
	ContentLength    string     `json:"contentLength"`
	Quality          string     `json:"quality"`
	Fps              int        `json:"fps,omitempty"`
	QualityLabel     string     `json:"qualityLabel,omitempty"`
	ProjectionType   string     `json:"projectionType"`
	AverageBitrate   int        `json:"averageBitrate"`
	ApproxDurationMs string     `json:"approxDurationMs"`
	ColorInfo        ColorInfo  `json:"colorInfo,omitempty"`
	HighReplication  bool       `json:"highReplication,omitempty"`
	AudioQuality     string     `json:"audioQuality,omitempty"`
	AudioSampleRate  string     `json:"audioSampleRate,omitempty"`
	AudioChannels    int        `json:"audioChannels,omitempty"`
	LoudnessDb       float64    `json:"loudnessDb,omitempty"`
	Sig              string     `json:"signatureCipher"`
}
type StreamingData struct {
	ExpiresInSeconds string            `json:"expiresInSeconds"`
	Formats          []Formats         `json:"formats"`
	AdaptiveFormats  []AdaptiveFormats `json:"adaptiveFormats"`
}

type DecipherOperation func([]byte) []byte

var Y = regexp.MustCompile(`^.*(youtu.be\/|v\/|e\/|u\/\w+\/|embed\/|v=)([^#\&\?]*).*`)

func YoutubeURL(URL string) string {
	YT_ := Y.FindAllStringSubmatch(URL, -1)
	if len(YT_) > 0 {
		if len(YT_[0]) > 2 {
			return YT_[0][2]
		}
	}
	return "Unknown"
}

func decrypt(cyphertext []byte, id string) ([]byte, error) {
	operations, err := parseDecipherOps(cyphertext, id)
	if err != nil {
		return nil, err
	}

	// apply operations
	bs := []byte(cyphertext)
	for _, op := range operations {
		bs = op(bs)
	}

	return bs, nil
}

const (
	jsvarStr   = "[a-zA-Z_\\$][a-zA-Z_0-9]*"
	reverseStr = ":function\\(a\\)\\{" +
		"(?:return )?a\\.reverse\\(\\)" +
		"\\}"
	spliceStr = ":function\\(a,b\\)\\{" +
		"a\\.splice\\(0,b\\)" +
		"\\}"
	swapStr = ":function\\(a,b\\)\\{" +
		"var c=a\\[0\\];a\\[0\\]=a\\[b(?:%a\\.length)?\\];a\\[b(?:%a\\.length)?\\]=c(?:;return a)?" +
		"\\}"
)

var (
	nFunctionNameRegexp = regexp.MustCompile("\\.get\\(\"n\"\\)\\)&&\\(b=([a-zA-Z0-9$]{0,3})\\[(\\d+)\\](.+)\\|\\|([a-zA-Z0-9]{0,3})")
	actionsObjRegexp    = regexp.MustCompile(fmt.Sprintf(
		"var (%s)=\\{((?:(?:%s%s|%s%s|%s%s),?\\n?)+)\\};", jsvarStr, jsvarStr, swapStr, jsvarStr, spliceStr, jsvarStr, reverseStr))
	actionsFuncRegexp = regexp.MustCompile(fmt.Sprintf(
		"function(?: %s)?\\(a\\)\\{"+
			"a=a\\.split\\(\"\"\\);\\s*"+
			"((?:(?:a=)?%s\\.%s\\(a,\\d+\\);)+)"+
			"return a\\.join\\(\"\"\\)"+
			"\\}", jsvarStr, jsvarStr, jsvarStr))
	reverseRegexp = regexp.MustCompile(fmt.Sprintf("(?m)(?:^|,)(%s)%s", jsvarStr, reverseStr))
	spliceRegexp  = regexp.MustCompile(fmt.Sprintf("(?m)(?:^|,)(%s)%s", jsvarStr, spliceStr))
	swapRegexp    = regexp.MustCompile(fmt.Sprintf("(?m)(?:^|,)(%s)%s", jsvarStr, swapStr))
)

func decryptNParam(config []byte, query url.Values) (url.Values, error) {
	// decrypt n-parameter
	nSig := query.Get("v")
	if nSig != "" {
		nDecoded, err := decodeNsig(config, nSig)
		if err != nil {
			return nil, fmt.Errorf("unable to decode nSig: %w", err)
		}
		query.Set("v", nDecoded)
	}

	return query, nil
}

func parseDecipherOps(config []byte, id string) (operations []DecipherOperation, err error) {
	config, _ = getPlayerConfig(id)
	objResult := actionsObjRegexp.FindSubmatch(config)
	funcResult := actionsFuncRegexp.FindSubmatch(config)
	if len(objResult) < 3 || len(funcResult) < 2 {
		return nil, fmt.Errorf("error parsing signature tokens (#obj=%d, #func=%d)", len(objResult), len(funcResult))
	}
	obj := objResult[1]
	objBody := objResult[2]
	funcBody := funcResult[1]
	var reverseKey, spliceKey, swapKey string
	if result := reverseRegexp.FindSubmatch(objBody); len(result) > 1 {
		reverseKey = string(result[1])
	}
	if result := spliceRegexp.FindSubmatch(objBody); len(result) > 1 {
		spliceKey = string(result[1])
	}
	if result := swapRegexp.FindSubmatch(objBody); len(result) > 1 {
		swapKey = string(result[1])
	}
	regex, err := regexp.Compile(fmt.Sprintf("(?:a=)?%s\\.(%s|%s|%s)\\(a,(\\d+)\\)", regexp.QuoteMeta(string(obj)), regexp.QuoteMeta(reverseKey), regexp.QuoteMeta(spliceKey), regexp.QuoteMeta(swapKey)))
	if err != nil {
		return nil, err
	}
	var ops []DecipherOperation
	for _, s := range regex.FindAllSubmatch(funcBody, -1) {
		switch string(s[1]) {
		case reverseKey:
			ops = append(ops, reverseFunc)
		case swapKey:
			arg, _ := strconv.Atoi(string(s[2]))
			ops = append(ops, newSwapFunc(arg))
		case spliceKey:
			arg, _ := strconv.Atoi(string(s[2]))
			ops = append(ops, newSpliceFunc(arg))
		}
	}
	return ops, nil
}

var basejsPattern = regexp.MustCompile(`(/s/player/\w+/player_ias.vflset/\w+/base.js)`)

func getPlayerConfig(videoID string) ([]byte, error) {
	embedURL := fmt.Sprintf("https://youtube.com/embed/%s?hl=en", videoID)
	req, _ := http.NewRequest("GET", embedURL, nil)
	req.Header.Set("Origin", "https://youtube.com")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	resp, _ := http.DefaultClient.Do(req)
	embedBody, _ := io.ReadAll(resp.Body)
	playerPath := string(basejsPattern.Find(embedBody))
	if playerPath == "" {
		return nil, errors.New("unable to find basejs URL in playerConfig")
	}
	reqa, _ := http.NewRequest("GET", "https://youtube.com"+playerPath, nil)
	reqa.Header.Set("Origin", "https://youtube.com")
	reqa.Header.Set("Sec-Fetch-Mode", "navigate")
	respa, _ := http.DefaultClient.Do(reqa)
	re, _ := io.ReadAll(respa.Body)
	return re, nil
}

func reverseFunc(bs []byte) []byte {
	l, r := 0, len(bs)-1
	for l < r {
		bs[l], bs[r] = bs[r], bs[l]
		l++
		r--
	}
	return bs
}

func newSwapFunc(arg int) DecipherOperation {
	return func(bs []byte) []byte {
		pos := arg % len(bs)
		bs[0], bs[pos] = bs[pos], bs[0]
		return bs
	}
}

func newSpliceFunc(pos int) DecipherOperation {
	return func(bs []byte) []byte {
		return bs[pos:]
	}
}

func getNFunction(config []byte) (string, error) {
	nameResult := nFunctionNameRegexp.FindSubmatch(config)
	if len(nameResult) == 0 {
		return "", errors.New("unable to extract n-function name")
	}

	var name string
	if idx, _ := strconv.Atoi(string(nameResult[2])); idx == 0 {
		name = string(nameResult[4])
	} else {
		name = string(nameResult[1])
	}

	return extraFunction(config, name)

}

func decodeNsig(config []byte, encoded string) (string, error) {
	fBody, err := getNFunction(config)
	if err != nil {
		return "", err
	}

	return evalJavascript(fBody, encoded)
}

func evalJavascript(jsFunction, arg string) (string, error) {
	const myName = "myFunction"

	vm := goja.New()
	_, err := vm.RunString(myName + "=" + jsFunction)
	if err != nil {
		return "", err
	}

	var output func(string) string
	err = vm.ExportTo(vm.Get(myName), &output)
	if err != nil {
		return "", err
	}

	return output(arg), nil
}

func extraFunction(config []byte, name string) (string, error) {
	// find the beginning of the function
	def := []byte(name + "=function(")
	start := bytes.Index(config, def)
	if start < 1 {
		return "", fmt.Errorf("unable to extract n-function body: looking for '%s'", def)
	}

	// start after the first curly bracket
	pos := start + bytes.IndexByte(config[start:], '{') + 1

	var strChar byte

	// find the bracket closing the function
	for brackets := 1; brackets > 0; pos++ {
		b := config[pos]
		switch b {
		case '{':
			if strChar == 0 {
				brackets++
			}
		case '}':
			if strChar == 0 {
				brackets--
			}
		case '`', '"', '\'':
			if config[pos-1] == '\\' && config[pos-2] != '\\' {
				continue
			}
			if strChar == 0 {
				strChar = b
			} else if strChar == b {
				strChar = 0
			}
		}
	}

	return string(config[start:pos]), nil
}

type YtPL struct {
	ResponseContext struct {
		ServiceTrackingParams []struct {
			Service string `json:"service"`
			Params  []struct {
				Key   string `json:"key"`
				Value string `json:"value"`
			} `json:"params"`
		} `json:"serviceTrackingParams"`
		MainAppWebResponseContext struct {
			DatasyncID    string `json:"datasyncId"`
			LoggedOut     bool   `json:"loggedOut"`
			TrackingParam string `json:"trackingParam"`
		} `json:"mainAppWebResponseContext"`
		WebResponseContextExtensionData struct {
			HasDecorated bool `json:"hasDecorated"`
		} `json:"webResponseContextExtensionData"`
	} `json:"responseContext"`
	Contents struct {
		TwoColumnBrowseResultsRenderer struct {
			Tabs []struct {
				TabRenderer struct {
					Selected       bool   `json:"selected"`
					TrackingParams string `json:"trackingParams"`
				} `json:"tabRenderer"`
			} `json:"tabs"`
		} `json:"twoColumnBrowseResultsRenderer"`
	} `json:"contents"`
	Alerts []struct {
		AlertWithButtonRenderer struct {
			Type string `json:"type"`
			Text struct {
				SimpleText string `json:"simpleText"`
			} `json:"text"`
			DismissButton struct {
				ButtonRenderer struct {
					Style      string `json:"style"`
					Size       string `json:"size"`
					IsDisabled bool   `json:"isDisabled"`
					Icon       struct {
						IconType string `json:"iconType"`
					} `json:"icon"`
					TrackingParams    string `json:"trackingParams"`
					AccessibilityData struct {
						AccessibilityData struct {
							Label string `json:"label"`
						} `json:"accessibilityData"`
					} `json:"accessibilityData"`
				} `json:"buttonRenderer"`
			} `json:"dismissButton"`
		} `json:"alertWithButtonRenderer"`
	} `json:"alerts"`
	Metadata struct {
		PlaylistMetadataRenderer struct {
			Title                  string `json:"title"`
			AndroidAppindexingLink string `json:"androidAppindexingLink"`
			IosAppindexingLink     string `json:"iosAppindexingLink"`
		} `json:"playlistMetadataRenderer"`
	} `json:"metadata"`
	TrackingParams string `json:"trackingParams"`
	Microformat    struct {
		MicroformatDataRenderer struct {
			URLCanonical string `json:"urlCanonical"`
			Title        string `json:"title"`
			Description  string `json:"description"`
			Thumbnail    struct {
				Thumbnails []struct {
					URL    string `json:"url"`
					Width  int    `json:"width"`
					Height int    `json:"height"`
				} `json:"thumbnails"`
			} `json:"thumbnail"`
			SiteName           string `json:"siteName"`
			AppName            string `json:"appName"`
			AndroidPackage     string `json:"androidPackage"`
			IosAppStoreID      string `json:"iosAppStoreId"`
			IosAppArguments    string `json:"iosAppArguments"`
			OgType             string `json:"ogType"`
			URLApplinksWeb     string `json:"urlApplinksWeb"`
			URLApplinksIos     string `json:"urlApplinksIos"`
			URLApplinksAndroid string `json:"urlApplinksAndroid"`
			URLTwitterIos      string `json:"urlTwitterIos"`
			URLTwitterAndroid  string `json:"urlTwitterAndroid"`
			TwitterCardType    string `json:"twitterCardType"`
			TwitterSiteHandle  string `json:"twitterSiteHandle"`
			SchemaDotOrgType   string `json:"schemaDotOrgType"`
			Noindex            bool   `json:"noindex"`
			Unlisted           bool   `json:"unlisted"`
			LinkAlternates     []struct {
				HrefURL string `json:"hrefUrl"`
			} `json:"linkAlternates"`
		} `json:"microformatDataRenderer"`
	} `json:"microformat"`
	OnResponseReceivedActions []struct {
		ClickTrackingParams           string `json:"clickTrackingParams"`
		AppendContinuationItemsAction struct {
			ContinuationItems []struct {
				PlaylistVideoRenderer struct {
					VideoID   string `json:"videoId"`
					Thumbnail struct {
						Thumbnails []struct {
							URL    string `json:"url"`
							Width  int    `json:"width"`
							Height int    `json:"height"`
						} `json:"thumbnails"`
					} `json:"thumbnail"`
					Title struct {
						Runs []struct {
							Text string `json:"text"`
						} `json:"runs"`
						Accessibility struct {
							AccessibilityData struct {
								Label string `json:"label"`
							} `json:"accessibilityData"`
						} `json:"accessibility"`
					} `json:"title"`
					Index struct {
						SimpleText string `json:"simpleText"`
					} `json:"index"`
					ShortBylineText struct {
						Runs []struct {
							Text               string `json:"text"`
							NavigationEndpoint struct {
								ClickTrackingParams string `json:"clickTrackingParams"`
								CommandMetadata     struct {
									WebCommandMetadata struct {
										URL         string `json:"url"`
										WebPageType string `json:"webPageType"`
										RootVe      int    `json:"rootVe"`
										APIURL      string `json:"apiUrl"`
									} `json:"webCommandMetadata"`
								} `json:"commandMetadata"`
								BrowseEndpoint struct {
									BrowseID         string `json:"browseId"`
									CanonicalBaseURL string `json:"canonicalBaseUrl"`
								} `json:"browseEndpoint"`
							} `json:"navigationEndpoint"`
						} `json:"runs"`
					} `json:"shortBylineText"`
					LengthText struct {
						Accessibility struct {
							AccessibilityData struct {
								Label string `json:"label"`
							} `json:"accessibilityData"`
						} `json:"accessibility"`
						SimpleText string `json:"simpleText"`
					} `json:"lengthText"`
					NavigationEndpoint struct {
						ClickTrackingParams string `json:"clickTrackingParams"`
						CommandMetadata     struct {
							WebCommandMetadata struct {
								URL         string `json:"url"`
								WebPageType string `json:"webPageType"`
								RootVe      int    `json:"rootVe"`
							} `json:"webCommandMetadata"`
						} `json:"commandMetadata"`
						WatchEndpoint struct {
							VideoID        string `json:"videoId"`
							PlaylistID     string `json:"playlistId"`
							Index          int    `json:"index"`
							Params         string `json:"params"`
							PlayerParams   string `json:"playerParams"`
							LoggingContext struct {
								VssLoggingContext struct {
									SerializedContextData string `json:"serializedContextData"`
								} `json:"vssLoggingContext"`
							} `json:"loggingContext"`
							WatchEndpointSupportedOnesieConfig struct {
								HTML5PlaybackOnesieConfig struct {
									CommonConfig struct {
										URL string `json:"url"`
									} `json:"commonConfig"`
								} `json:"html5PlaybackOnesieConfig"`
							} `json:"watchEndpointSupportedOnesieConfig"`
						} `json:"watchEndpoint"`
					} `json:"navigationEndpoint"`
					SetVideoID     string `json:"setVideoId"`
					LengthSeconds  string `json:"lengthSeconds"`
					TrackingParams string `json:"trackingParams"`
					IsPlayable     bool   `json:"isPlayable"`
					Menu           struct {
						MenuRenderer struct {
							Items []struct {
								MenuServiceItemRenderer struct {
									Text struct {
										Runs []struct {
											Text string `json:"text"`
										} `json:"runs"`
									} `json:"text"`
									Icon struct {
										IconType string `json:"iconType"`
									} `json:"icon"`
									ServiceEndpoint struct {
										ClickTrackingParams string `json:"clickTrackingParams"`
										CommandMetadata     struct {
											WebCommandMetadata struct {
												SendPost bool `json:"sendPost"`
											} `json:"webCommandMetadata"`
										} `json:"commandMetadata"`
										SignalServiceEndpoint struct {
											Signal  string `json:"signal"`
											Actions []struct {
												ClickTrackingParams  string `json:"clickTrackingParams"`
												AddToPlaylistCommand struct {
													OpenMiniplayer      bool   `json:"openMiniplayer"`
													VideoID             string `json:"videoId"`
													ListType            string `json:"listType"`
													OnCreateListCommand struct {
														ClickTrackingParams string `json:"clickTrackingParams"`
														CommandMetadata     struct {
															WebCommandMetadata struct {
																SendPost bool   `json:"sendPost"`
																APIURL   string `json:"apiUrl"`
															} `json:"webCommandMetadata"`
														} `json:"commandMetadata"`
														CreatePlaylistServiceEndpoint struct {
															VideoIds []string `json:"videoIds"`
															Params   string   `json:"params"`
														} `json:"createPlaylistServiceEndpoint"`
													} `json:"onCreateListCommand"`
													VideoIds []string `json:"videoIds"`
												} `json:"addToPlaylistCommand"`
											} `json:"actions"`
										} `json:"signalServiceEndpoint"`
									} `json:"serviceEndpoint"`
									TrackingParams string `json:"trackingParams"`
								} `json:"menuServiceItemRenderer,omitempty"`
								MenuServiceItemDownloadRenderer struct {
									ServiceEndpoint struct {
										ClickTrackingParams  string `json:"clickTrackingParams"`
										OfflineVideoEndpoint struct {
											VideoID      string `json:"videoId"`
											OnAddCommand struct {
												ClickTrackingParams      string `json:"clickTrackingParams"`
												GetDownloadActionCommand struct {
													VideoID string `json:"videoId"`
													Params  string `json:"params"`
												} `json:"getDownloadActionCommand"`
											} `json:"onAddCommand"`
										} `json:"offlineVideoEndpoint"`
									} `json:"serviceEndpoint"`
									TrackingParams string `json:"trackingParams"`
								} `json:"menuServiceItemDownloadRenderer,omitempty"`
							} `json:"items"`
							TrackingParams string `json:"trackingParams"`
							Accessibility  struct {
								AccessibilityData struct {
									Label string `json:"label"`
								} `json:"accessibilityData"`
							} `json:"accessibility"`
						} `json:"menuRenderer"`
					} `json:"menu"`
					ThumbnailOverlays []struct {
						ThumbnailOverlayPlaybackStatusRenderer struct {
							Texts []struct {
								Runs []struct {
									Text string `json:"text"`
								} `json:"runs"`
							} `json:"texts"`
						} `json:"thumbnailOverlayPlaybackStatusRenderer,omitempty"`
						ThumbnailOverlayResumePlaybackRenderer struct {
							PercentDurationWatched int `json:"percentDurationWatched"`
						} `json:"thumbnailOverlayResumePlaybackRenderer,omitempty"`
						ThumbnailOverlayTimeStatusRenderer struct {
							Text struct {
								Accessibility struct {
									AccessibilityData struct {
										Label string `json:"label"`
									} `json:"accessibilityData"`
								} `json:"accessibility"`
								SimpleText string `json:"simpleText"`
							} `json:"text"`
							Style string `json:"style"`
						} `json:"thumbnailOverlayTimeStatusRenderer,omitempty"`
						ThumbnailOverlayNowPlayingRenderer struct {
							Text struct {
								Runs []struct {
									Text string `json:"text"`
								} `json:"runs"`
							} `json:"text"`
						} `json:"thumbnailOverlayNowPlayingRenderer,omitempty"`
					} `json:"thumbnailOverlays"`
					VideoInfo struct {
						Runs []struct {
							Text string `json:"text"`
						} `json:"runs"`
					} `json:"videoInfo"`
				} `json:"playlistVideoRenderer"`
			} `json:"continuationItems"`
			TargetID string `json:"targetId"`
		} `json:"appendContinuationItemsAction"`
	} `json:"onResponseReceivedActions"`
	Sidebar struct {
		PlaylistSidebarRenderer struct {
			Items []struct {
				PlaylistSidebarPrimaryInfoRenderer struct {
					ThumbnailRenderer struct {
						PlaylistVideoThumbnailRenderer struct {
							Thumbnail struct {
								Thumbnails []struct {
									URL    string `json:"url"`
									Width  int    `json:"width"`
									Height int    `json:"height"`
								} `json:"thumbnails"`
							} `json:"thumbnail"`
							TrackingParams string `json:"trackingParams"`
						} `json:"playlistVideoThumbnailRenderer"`
					} `json:"thumbnailRenderer"`
					Stats []struct {
						Runs []struct {
							Text string `json:"text"`
						} `json:"runs,omitempty"`
						SimpleText string `json:"simpleText,omitempty"`
					} `json:"stats"`
					Menu struct {
						MenuRenderer struct {
							Items []struct {
								MenuServiceItemRenderer struct {
									Text struct {
										Runs []struct {
											Text string `json:"text"`
										} `json:"runs"`
									} `json:"text"`
									Icon struct {
										IconType string `json:"iconType"`
									} `json:"icon"`
									ServiceEndpoint struct {
										ClickTrackingParams string `json:"clickTrackingParams"`
										CommandMetadata     struct {
											WebCommandMetadata struct {
												SendPost bool `json:"sendPost"`
											} `json:"webCommandMetadata"`
										} `json:"commandMetadata"`
										SignalServiceEndpoint struct {
											Signal  string `json:"signal"`
											Actions []struct {
												ClickTrackingParams             string `json:"clickTrackingParams"`
												OpenOnePickAddVideoModalCommand struct {
													ListID            string `json:"listId"`
													ModalTitle        string `json:"modalTitle"`
													SelectButtonLabel string `json:"selectButtonLabel"`
												} `json:"openOnePickAddVideoModalCommand"`
											} `json:"actions"`
										} `json:"signalServiceEndpoint"`
									} `json:"serviceEndpoint"`
									TrackingParams string `json:"trackingParams"`
								} `json:"menuServiceItemRenderer,omitempty"`
								MenuNavigationItemRenderer struct {
									Text struct {
										SimpleText string `json:"simpleText"`
									} `json:"text"`
									Icon struct {
										IconType string `json:"iconType"`
									} `json:"icon"`
									NavigationEndpoint struct {
										ClickTrackingParams string `json:"clickTrackingParams"`
										CommandMetadata     struct {
											WebCommandMetadata struct {
												URL         string `json:"url"`
												WebPageType string `json:"webPageType"`
												RootVe      int    `json:"rootVe"`
												APIURL      string `json:"apiUrl"`
											} `json:"webCommandMetadata"`
										} `json:"commandMetadata"`
										BrowseEndpoint struct {
											BrowseID       string `json:"browseId"`
											Params         string `json:"params"`
											Nofollow       bool   `json:"nofollow"`
											NavigationType string `json:"navigationType"`
										} `json:"browseEndpoint"`
									} `json:"navigationEndpoint"`
									TrackingParams string `json:"trackingParams"`
								} `json:"menuNavigationItemRenderer,omitempty"`
							} `json:"items"`
							TrackingParams  string `json:"trackingParams"`
							TopLevelButtons []struct {
								ButtonRenderer struct {
									Style      string `json:"style"`
									Size       string `json:"size"`
									IsDisabled bool   `json:"isDisabled"`
									Icon       struct {
										IconType string `json:"iconType"`
									} `json:"icon"`
									NavigationEndpoint struct {
										ClickTrackingParams string `json:"clickTrackingParams"`
										CommandMetadata     struct {
											WebCommandMetadata struct {
												URL         string `json:"url"`
												WebPageType string `json:"webPageType"`
												RootVe      int    `json:"rootVe"`
											} `json:"webCommandMetadata"`
										} `json:"commandMetadata"`
										WatchEndpoint struct {
											VideoID        string `json:"videoId"`
											PlaylistID     string `json:"playlistId"`
											Params         string `json:"params"`
											PlayerParams   string `json:"playerParams"`
											LoggingContext struct {
												VssLoggingContext struct {
													SerializedContextData string `json:"serializedContextData"`
												} `json:"vssLoggingContext"`
											} `json:"loggingContext"`
											WatchEndpointSupportedOnesieConfig struct {
												HTML5PlaybackOnesieConfig struct {
													CommonConfig struct {
														URL string `json:"url"`
													} `json:"commonConfig"`
												} `json:"html5PlaybackOnesieConfig"`
											} `json:"watchEndpointSupportedOnesieConfig"`
										} `json:"watchEndpoint"`
									} `json:"navigationEndpoint"`
									Accessibility struct {
										Label string `json:"label"`
									} `json:"accessibility"`
									Tooltip        string `json:"tooltip"`
									TrackingParams string `json:"trackingParams"`
								} `json:"buttonRenderer,omitempty"`
							} `json:"topLevelButtons"`
							Accessibility struct {
								AccessibilityData struct {
									Label string `json:"label"`
								} `json:"accessibilityData"`
							} `json:"accessibility"`
							TargetID string `json:"targetId"`
						} `json:"menuRenderer"`
					} `json:"menu"`
					ThumbnailOverlays []struct {
						ThumbnailOverlaySidePanelRenderer struct {
							Text struct {
								SimpleText string `json:"simpleText"`
							} `json:"text"`
							Icon struct {
								IconType string `json:"iconType"`
							} `json:"icon"`
						} `json:"thumbnailOverlaySidePanelRenderer"`
					} `json:"thumbnailOverlays"`
					NavigationEndpoint struct {
						ClickTrackingParams string `json:"clickTrackingParams"`
						CommandMetadata     struct {
							WebCommandMetadata struct {
								URL         string `json:"url"`
								WebPageType string `json:"webPageType"`
								RootVe      int    `json:"rootVe"`
							} `json:"webCommandMetadata"`
						} `json:"commandMetadata"`
						WatchEndpoint struct {
							VideoID        string `json:"videoId"`
							PlaylistID     string `json:"playlistId"`
							PlayerParams   string `json:"playerParams"`
							LoggingContext struct {
								VssLoggingContext struct {
									SerializedContextData string `json:"serializedContextData"`
								} `json:"vssLoggingContext"`
							} `json:"loggingContext"`
							WatchEndpointSupportedOnesieConfig struct {
								HTML5PlaybackOnesieConfig struct {
									CommonConfig struct {
										URL string `json:"url"`
									} `json:"commonConfig"`
								} `json:"html5PlaybackOnesieConfig"`
							} `json:"watchEndpointSupportedOnesieConfig"`
						} `json:"watchEndpoint"`
					} `json:"navigationEndpoint"`
					ShowMoreText struct {
						Runs []struct {
							Text string `json:"text"`
						} `json:"runs"`
					} `json:"showMoreText"`
					TitleForm struct {
						InlineFormRenderer struct {
							FormField struct {
								TextInputFormFieldRenderer struct {
									Label struct {
										Runs []struct {
											Text string `json:"text"`
										} `json:"runs"`
									} `json:"label"`
									Value             string `json:"value"`
									MaxCharacterLimit int    `json:"maxCharacterLimit"`
									Key               string `json:"key"`
									OnChange          struct {
										ClickTrackingParams string `json:"clickTrackingParams"`
										CommandMetadata     struct {
											WebCommandMetadata struct {
												SendPost bool   `json:"sendPost"`
												APIURL   string `json:"apiUrl"`
											} `json:"webCommandMetadata"`
										} `json:"commandMetadata"`
										PlaylistEditEndpoint struct {
											PlaylistID string `json:"playlistId"`
											Actions    []struct {
												Action       string `json:"action"`
												PlaylistName string `json:"playlistName"`
											} `json:"actions"`
										} `json:"playlistEditEndpoint"`
									} `json:"onChange"`
									ValidValueRegexp         string `json:"validValueRegexp"`
									InvalidValueErrorMessage struct {
										Runs []struct {
											Text string `json:"text"`
										} `json:"runs"`
									} `json:"invalidValueErrorMessage"`
									Required bool `json:"required"`
								} `json:"textInputFormFieldRenderer"`
							} `json:"formField"`
							EditButton struct {
								ButtonRenderer struct {
									Style      string `json:"style"`
									Size       string `json:"size"`
									IsDisabled bool   `json:"isDisabled"`
									Icon       struct {
										IconType string `json:"iconType"`
									} `json:"icon"`
									Accessibility struct {
										Label string `json:"label"`
									} `json:"accessibility"`
									Tooltip        string `json:"tooltip"`
									TrackingParams string `json:"trackingParams"`
								} `json:"buttonRenderer"`
							} `json:"editButton"`
							SaveButton struct {
								ButtonRenderer struct {
									Style      string `json:"style"`
									Size       string `json:"size"`
									IsDisabled bool   `json:"isDisabled"`
									Text       struct {
										Runs []struct {
											Text string `json:"text"`
										} `json:"runs"`
									} `json:"text"`
									Accessibility struct {
										Label string `json:"label"`
									} `json:"accessibility"`
									TrackingParams string `json:"trackingParams"`
								} `json:"buttonRenderer"`
							} `json:"saveButton"`
							CancelButton struct {
								ButtonRenderer struct {
									Style      string `json:"style"`
									Size       string `json:"size"`
									IsDisabled bool   `json:"isDisabled"`
									Text       struct {
										Runs []struct {
											Text string `json:"text"`
										} `json:"runs"`
									} `json:"text"`
									Accessibility struct {
										Label string `json:"label"`
									} `json:"accessibility"`
									TrackingParams string `json:"trackingParams"`
								} `json:"buttonRenderer"`
							} `json:"cancelButton"`
							TextDisplayed struct {
								SimpleText string `json:"simpleText"`
							} `json:"textDisplayed"`
							Style string `json:"style"`
						} `json:"inlineFormRenderer"`
					} `json:"titleForm"`
					DescriptionForm struct {
						InlineFormRenderer struct {
							FormField struct {
								TextInputFormFieldRenderer struct {
									Label struct {
										Runs []struct {
											Text string `json:"text"`
										} `json:"runs"`
									} `json:"label"`
									Value             string `json:"value"`
									MaxCharacterLimit int    `json:"maxCharacterLimit"`
									Key               string `json:"key"`
									OnChange          struct {
										ClickTrackingParams string `json:"clickTrackingParams"`
										CommandMetadata     struct {
											WebCommandMetadata struct {
												SendPost bool   `json:"sendPost"`
												APIURL   string `json:"apiUrl"`
											} `json:"webCommandMetadata"`
										} `json:"commandMetadata"`
										PlaylistEditEndpoint struct {
											PlaylistID string `json:"playlistId"`
											Actions    []struct {
												Action              string `json:"action"`
												PlaylistDescription string `json:"playlistDescription"`
											} `json:"actions"`
											Params string `json:"params"`
										} `json:"playlistEditEndpoint"`
									} `json:"onChange"`
									ValidValueRegexp         string `json:"validValueRegexp"`
									InvalidValueErrorMessage struct {
										Runs []struct {
											Text string `json:"text"`
										} `json:"runs"`
									} `json:"invalidValueErrorMessage"`
									IsMultiline bool `json:"isMultiline"`
								} `json:"textInputFormFieldRenderer"`
							} `json:"formField"`
							EditButton struct {
								ButtonRenderer struct {
									Style      string `json:"style"`
									Size       string `json:"size"`
									IsDisabled bool   `json:"isDisabled"`
									Icon       struct {
										IconType string `json:"iconType"`
									} `json:"icon"`
									Accessibility struct {
										Label string `json:"label"`
									} `json:"accessibility"`
									Tooltip        string `json:"tooltip"`
									TrackingParams string `json:"trackingParams"`
								} `json:"buttonRenderer"`
							} `json:"editButton"`
							SaveButton struct {
								ButtonRenderer struct {
									Style      string `json:"style"`
									Size       string `json:"size"`
									IsDisabled bool   `json:"isDisabled"`
									Text       struct {
										Runs []struct {
											Text string `json:"text"`
										} `json:"runs"`
									} `json:"text"`
									Accessibility struct {
										Label string `json:"label"`
									} `json:"accessibility"`
									TrackingParams string `json:"trackingParams"`
								} `json:"buttonRenderer"`
							} `json:"saveButton"`
							CancelButton struct {
								ButtonRenderer struct {
									Style      string `json:"style"`
									Size       string `json:"size"`
									IsDisabled bool   `json:"isDisabled"`
									Text       struct {
										Runs []struct {
											Text string `json:"text"`
										} `json:"runs"`
									} `json:"text"`
									Accessibility struct {
										Label string `json:"label"`
									} `json:"accessibility"`
									TrackingParams string `json:"trackingParams"`
								} `json:"buttonRenderer"`
							} `json:"cancelButton"`
							Style       string `json:"style"`
							Placeholder struct {
								Runs []struct {
									Text string `json:"text"`
								} `json:"runs"`
							} `json:"placeholder"`
						} `json:"inlineFormRenderer"`
					} `json:"descriptionForm"`
					PrivacyForm struct {
						DropdownFormFieldRenderer struct {
							Dropdown struct {
								DropdownRenderer struct {
									Entries []struct {
										PrivacyDropdownItemRenderer struct {
											Label struct {
												Runs []struct {
													Text string `json:"text"`
												} `json:"runs"`
											} `json:"label"`
											Icon struct {
												IconType string `json:"iconType"`
											} `json:"icon"`
											Description struct {
												Runs []struct {
													Text string `json:"text"`
												} `json:"runs"`
											} `json:"description"`
											Int32Value    int  `json:"int32Value"`
											IsSelected    bool `json:"isSelected"`
											Accessibility struct {
												Label string `json:"label"`
											} `json:"accessibility"`
										} `json:"privacyDropdownItemRenderer"`
									} `json:"entries"`
								} `json:"dropdownRenderer"`
							} `json:"dropdown"`
							Key      string `json:"key"`
							OnChange struct {
								ClickTrackingParams string `json:"clickTrackingParams"`
								CommandMetadata     struct {
									WebCommandMetadata struct {
										SendPost bool   `json:"sendPost"`
										APIURL   string `json:"apiUrl"`
									} `json:"webCommandMetadata"`
								} `json:"commandMetadata"`
								PlaylistEditEndpoint struct {
									PlaylistID string `json:"playlistId"`
									Actions    []struct {
										Action          string `json:"action"`
										PlaylistPrivacy string `json:"playlistPrivacy"`
									} `json:"actions"`
									Params string `json:"params"`
								} `json:"playlistEditEndpoint"`
							} `json:"onChange"`
						} `json:"dropdownFormFieldRenderer"`
					} `json:"privacyForm"`
				} `json:"playlistSidebarPrimaryInfoRenderer,omitempty"`
				PlaylistSidebarSecondaryInfoRenderer struct {
					VideoOwner struct {
						VideoOwnerRenderer struct {
							Thumbnail struct {
								Thumbnails []struct {
									URL    string `json:"url"`
									Width  int    `json:"width"`
									Height int    `json:"height"`
								} `json:"thumbnails"`
							} `json:"thumbnail"`
							Title struct {
								Runs []struct {
									Text               string `json:"text"`
									NavigationEndpoint struct {
										ClickTrackingParams string `json:"clickTrackingParams"`
										CommandMetadata     struct {
											WebCommandMetadata struct {
												URL         string `json:"url"`
												WebPageType string `json:"webPageType"`
												RootVe      int    `json:"rootVe"`
												APIURL      string `json:"apiUrl"`
											} `json:"webCommandMetadata"`
										} `json:"commandMetadata"`
										BrowseEndpoint struct {
											BrowseID         string `json:"browseId"`
											CanonicalBaseURL string `json:"canonicalBaseUrl"`
										} `json:"browseEndpoint"`
									} `json:"navigationEndpoint"`
								} `json:"runs"`
							} `json:"title"`
							NavigationEndpoint struct {
								ClickTrackingParams string `json:"clickTrackingParams"`
								CommandMetadata     struct {
									WebCommandMetadata struct {
										URL         string `json:"url"`
										WebPageType string `json:"webPageType"`
										RootVe      int    `json:"rootVe"`
										APIURL      string `json:"apiUrl"`
									} `json:"webCommandMetadata"`
								} `json:"commandMetadata"`
								BrowseEndpoint struct {
									BrowseID         string `json:"browseId"`
									CanonicalBaseURL string `json:"canonicalBaseUrl"`
								} `json:"browseEndpoint"`
							} `json:"navigationEndpoint"`
							TrackingParams string `json:"trackingParams"`
						} `json:"videoOwnerRenderer"`
					} `json:"videoOwner"`
				} `json:"playlistSidebarSecondaryInfoRenderer,omitempty"`
			} `json:"items"`
			TrackingParams string `json:"trackingParams"`
		} `json:"playlistSidebarRenderer"`
	} `json:"sidebar"`
}

type YTPageConfig struct {
	ResponseContext struct {
		ServiceTrackingParams []struct {
			Service string `json:"service"`
			Params  []struct {
				Key   string `json:"key"`
				Value string `json:"value"`
			} `json:"params"`
		} `json:"serviceTrackingParams"`
		MainAppWebResponseContext struct {
			LoggedOut     bool   `json:"loggedOut"`
			TrackingParam string `json:"trackingParam"`
		} `json:"mainAppWebResponseContext"`
		WebResponseContextExtensionData struct {
			YtConfigData struct {
				VisitorData           string `json:"visitorData"`
				RootVisualElementType int    `json:"rootVisualElementType"`
			} `json:"ytConfigData"`
			HasDecorated bool `json:"hasDecorated"`
		} `json:"webResponseContextExtensionData"`
	} `json:"responseContext"`
	Contents struct {
		TwoColumnBrowseResultsRenderer struct {
			Tabs []struct {
				TabRenderer struct {
					Selected bool `json:"selected"`
					Content  struct {
						SectionListRenderer struct {
							Contents []struct {
								ItemSectionRenderer struct {
									Contents []struct {
										PlaylistVideoListRenderer struct {
											Contents []struct {
												PlaylistVideoRenderer struct {
													VideoID   string `json:"videoId"`
													Thumbnail struct {
														Thumbnails []struct {
															URL    string `json:"url"`
															Width  int    `json:"width"`
															Height int    `json:"height"`
														} `json:"thumbnails"`
													} `json:"thumbnail"`
													Title struct {
														Runs []struct {
															Text string `json:"text"`
														} `json:"runs"`
														Accessibility struct {
															AccessibilityData struct {
																Label string `json:"label"`
															} `json:"accessibilityData"`
														} `json:"accessibility"`
													} `json:"title"`
													Index struct {
														SimpleText string `json:"simpleText"`
													} `json:"index"`
													ShortBylineText struct {
														Runs []struct {
															Text               string `json:"text"`
															NavigationEndpoint struct {
																ClickTrackingParams string `json:"clickTrackingParams"`
																CommandMetadata     struct {
																	WebCommandMetadata struct {
																		URL         string `json:"url"`
																		WebPageType string `json:"webPageType"`
																		RootVe      int    `json:"rootVe"`
																		APIURL      string `json:"apiUrl"`
																	} `json:"webCommandMetadata"`
																} `json:"commandMetadata"`
																BrowseEndpoint struct {
																	BrowseID         string `json:"browseId"`
																	CanonicalBaseURL string `json:"canonicalBaseUrl"`
																} `json:"browseEndpoint"`
															} `json:"navigationEndpoint"`
														} `json:"runs"`
													} `json:"shortBylineText"`
													LengthText struct {
														Accessibility struct {
															AccessibilityData struct {
																Label string `json:"label"`
															} `json:"accessibilityData"`
														} `json:"accessibility"`
														SimpleText string `json:"simpleText"`
													} `json:"lengthText"`
													NavigationEndpoint struct {
														ClickTrackingParams string `json:"clickTrackingParams"`
														CommandMetadata     struct {
															WebCommandMetadata struct {
																URL         string `json:"url"`
																WebPageType string `json:"webPageType"`
																RootVe      int    `json:"rootVe"`
															} `json:"webCommandMetadata"`
														} `json:"commandMetadata"`
														WatchEndpoint struct {
															VideoID        string `json:"videoId"`
															PlaylistID     string `json:"playlistId"`
															Index          int    `json:"index"`
															Params         string `json:"params"`
															PlayerParams   string `json:"playerParams"`
															LoggingContext struct {
																VssLoggingContext struct {
																	SerializedContextData string `json:"serializedContextData"`
																} `json:"vssLoggingContext"`
															} `json:"loggingContext"`
															WatchEndpointSupportedOnesieConfig struct {
																HTML5PlaybackOnesieConfig struct {
																	CommonConfig struct {
																		URL string `json:"url"`
																	} `json:"commonConfig"`
																} `json:"html5PlaybackOnesieConfig"`
															} `json:"watchEndpointSupportedOnesieConfig"`
														} `json:"watchEndpoint"`
													} `json:"navigationEndpoint"`
													LengthSeconds  string `json:"lengthSeconds"`
													TrackingParams string `json:"trackingParams"`
													IsPlayable     bool   `json:"isPlayable"`
													Menu           struct {
														MenuRenderer struct {
															Items []struct {
																MenuServiceItemRenderer struct {
																	Text struct {
																		Runs []struct {
																			Text string `json:"text"`
																		} `json:"runs"`
																	} `json:"text"`
																	Icon struct {
																		IconType string `json:"iconType"`
																	} `json:"icon"`
																	ServiceEndpoint struct {
																		ClickTrackingParams string `json:"clickTrackingParams"`
																		CommandMetadata     struct {
																			WebCommandMetadata struct {
																				SendPost bool `json:"sendPost"`
																			} `json:"webCommandMetadata"`
																		} `json:"commandMetadata"`
																		SignalServiceEndpoint struct {
																			Signal  string `json:"signal"`
																			Actions []struct {
																				ClickTrackingParams  string `json:"clickTrackingParams"`
																				AddToPlaylistCommand struct {
																					OpenMiniplayer      bool   `json:"openMiniplayer"`
																					VideoID             string `json:"videoId"`
																					ListType            string `json:"listType"`
																					OnCreateListCommand struct {
																						ClickTrackingParams string `json:"clickTrackingParams"`
																						CommandMetadata     struct {
																							WebCommandMetadata struct {
																								SendPost bool   `json:"sendPost"`
																								APIURL   string `json:"apiUrl"`
																							} `json:"webCommandMetadata"`
																						} `json:"commandMetadata"`
																						CreatePlaylistServiceEndpoint struct {
																							VideoIds []string `json:"videoIds"`
																							Params   string   `json:"params"`
																						} `json:"createPlaylistServiceEndpoint"`
																					} `json:"onCreateListCommand"`
																					VideoIds []string `json:"videoIds"`
																				} `json:"addToPlaylistCommand"`
																			} `json:"actions"`
																		} `json:"signalServiceEndpoint"`
																	} `json:"serviceEndpoint"`
																	TrackingParams string `json:"trackingParams"`
																} `json:"menuServiceItemRenderer,omitempty"`
															} `json:"items"`
															TrackingParams string `json:"trackingParams"`
															Accessibility  struct {
																AccessibilityData struct {
																	Label string `json:"label"`
																} `json:"accessibilityData"`
															} `json:"accessibility"`
														} `json:"menuRenderer"`
													} `json:"menu"`
													ThumbnailOverlays []struct {
														ThumbnailOverlayTimeStatusRenderer struct {
															Text struct {
																Accessibility struct {
																	AccessibilityData struct {
																		Label string `json:"label"`
																	} `json:"accessibilityData"`
																} `json:"accessibility"`
																SimpleText string `json:"simpleText"`
															} `json:"text"`
															Style string `json:"style"`
														} `json:"thumbnailOverlayTimeStatusRenderer,omitempty"`
														ThumbnailOverlayNowPlayingRenderer struct {
															Text struct {
																Runs []struct {
																	Text string `json:"text"`
																} `json:"runs"`
															} `json:"text"`
														} `json:"thumbnailOverlayNowPlayingRenderer,omitempty"`
													} `json:"thumbnailOverlays"`
													VideoInfo struct {
														Runs []struct {
															Text string `json:"text"`
														} `json:"runs"`
													} `json:"videoInfo"`
												} `json:"playlistVideoRenderer,omitempty"`
												ContinuationItemRenderer struct {
													Trigger              string `json:"trigger"`
													ContinuationEndpoint struct {
														ClickTrackingParams string `json:"clickTrackingParams"`
														CommandMetadata     struct {
															WebCommandMetadata struct {
																SendPost bool   `json:"sendPost"`
																APIURL   string `json:"apiUrl"`
															} `json:"webCommandMetadata"`
														} `json:"commandMetadata"`
														ContinuationCommand struct {
															Token   string `json:"token"`
															Request string `json:"request"`
														} `json:"continuationCommand"`
													} `json:"continuationEndpoint"`
												} `json:"continuationItemRenderer,omitempty"`
											} `json:"contents"`
											PlaylistID     string `json:"playlistId"`
											IsEditable     bool   `json:"isEditable"`
											CanReorder     bool   `json:"canReorder"`
											TrackingParams string `json:"trackingParams"`
											TargetID       string `json:"targetId"`
										} `json:"playlistVideoListRenderer"`
									} `json:"contents"`
									TrackingParams string `json:"trackingParams"`
								} `json:"itemSectionRenderer"`
							} `json:"contents"`
							TrackingParams string `json:"trackingParams"`
						} `json:"sectionListRenderer"`
					} `json:"content"`
					TrackingParams string `json:"trackingParams"`
				} `json:"tabRenderer"`
			} `json:"tabs"`
		} `json:"twoColumnBrowseResultsRenderer"`
	} `json:"contents"`
	Header struct {
		PlaylistHeaderRenderer struct {
			PlaylistID string `json:"playlistId"`
			Title      struct {
				SimpleText string `json:"simpleText"`
			} `json:"title"`
			NumVideosText struct {
				Runs []struct {
					Text string `json:"text"`
				} `json:"runs"`
			} `json:"numVideosText"`
			DescriptionText struct {
			} `json:"descriptionText"`
			OwnerText struct {
				Runs []struct {
					Text               string `json:"text"`
					NavigationEndpoint struct {
						ClickTrackingParams string `json:"clickTrackingParams"`
						CommandMetadata     struct {
							WebCommandMetadata struct {
								URL         string `json:"url"`
								WebPageType string `json:"webPageType"`
								RootVe      int    `json:"rootVe"`
								APIURL      string `json:"apiUrl"`
							} `json:"webCommandMetadata"`
						} `json:"commandMetadata"`
						BrowseEndpoint struct {
							BrowseID         string `json:"browseId"`
							CanonicalBaseURL string `json:"canonicalBaseUrl"`
						} `json:"browseEndpoint"`
					} `json:"navigationEndpoint"`
				} `json:"runs"`
			} `json:"ownerText"`
			ViewCountText struct {
				SimpleText string `json:"simpleText"`
			} `json:"viewCountText"`
			ShareData struct {
				CanShare bool `json:"canShare"`
			} `json:"shareData"`
			IsEditable    bool   `json:"isEditable"`
			Privacy       string `json:"privacy"`
			OwnerEndpoint struct {
				ClickTrackingParams string `json:"clickTrackingParams"`
				CommandMetadata     struct {
					WebCommandMetadata struct {
						URL         string `json:"url"`
						WebPageType string `json:"webPageType"`
						RootVe      int    `json:"rootVe"`
						APIURL      string `json:"apiUrl"`
					} `json:"webCommandMetadata"`
				} `json:"commandMetadata"`
				BrowseEndpoint struct {
					BrowseID         string `json:"browseId"`
					CanonicalBaseURL string `json:"canonicalBaseUrl"`
				} `json:"browseEndpoint"`
			} `json:"ownerEndpoint"`
			EditableDetails struct {
				CanDelete bool `json:"canDelete"`
			} `json:"editableDetails"`
			TrackingParams   string `json:"trackingParams"`
			ServiceEndpoints []struct {
				ClickTrackingParams string `json:"clickTrackingParams"`
				CommandMetadata     struct {
					WebCommandMetadata struct {
						SendPost bool   `json:"sendPost"`
						APIURL   string `json:"apiUrl"`
					} `json:"webCommandMetadata"`
				} `json:"commandMetadata"`
				PlaylistEditEndpoint struct {
					Actions []struct {
						Action           string `json:"action"`
						SourcePlaylistID string `json:"sourcePlaylistId"`
					} `json:"actions"`
				} `json:"playlistEditEndpoint"`
			} `json:"serviceEndpoints"`
			Stats []struct {
				Runs []struct {
					Text string `json:"text"`
				} `json:"runs,omitempty"`
				SimpleText string `json:"simpleText,omitempty"`
			} `json:"stats"`
			BriefStats []struct {
				Runs []struct {
					Text string `json:"text"`
				} `json:"runs"`
			} `json:"briefStats"`
			PlaylistHeaderBanner struct {
				HeroPlaylistThumbnailRenderer struct {
					Thumbnail struct {
						Thumbnails []struct {
							URL    string `json:"url"`
							Width  int    `json:"width"`
							Height int    `json:"height"`
						} `json:"thumbnails"`
					} `json:"thumbnail"`
					MaxRatio       float64 `json:"maxRatio"`
					TrackingParams string  `json:"trackingParams"`
					OnTap          struct {
						ClickTrackingParams string `json:"clickTrackingParams"`
						CommandMetadata     struct {
							WebCommandMetadata struct {
								URL         string `json:"url"`
								WebPageType string `json:"webPageType"`
								RootVe      int    `json:"rootVe"`
							} `json:"webCommandMetadata"`
						} `json:"commandMetadata"`
						WatchEndpoint struct {
							VideoID        string `json:"videoId"`
							PlaylistID     string `json:"playlistId"`
							PlayerParams   string `json:"playerParams"`
							LoggingContext struct {
								VssLoggingContext struct {
									SerializedContextData string `json:"serializedContextData"`
								} `json:"vssLoggingContext"`
							} `json:"loggingContext"`
							WatchEndpointSupportedOnesieConfig struct {
								HTML5PlaybackOnesieConfig struct {
									CommonConfig struct {
										URL string `json:"url"`
									} `json:"commonConfig"`
								} `json:"html5PlaybackOnesieConfig"`
							} `json:"watchEndpointSupportedOnesieConfig"`
						} `json:"watchEndpoint"`
					} `json:"onTap"`
					ThumbnailOverlays struct {
						ThumbnailOverlayHoverTextRenderer struct {
							Text struct {
								SimpleText string `json:"simpleText"`
							} `json:"text"`
							Icon struct {
								IconType string `json:"iconType"`
							} `json:"icon"`
						} `json:"thumbnailOverlayHoverTextRenderer"`
					} `json:"thumbnailOverlays"`
				} `json:"heroPlaylistThumbnailRenderer"`
			} `json:"playlistHeaderBanner"`
			SaveButton struct {
				ToggleButtonRenderer struct {
					Style struct {
						StyleType string `json:"styleType"`
					} `json:"style"`
					Size struct {
						SizeType string `json:"sizeType"`
					} `json:"size"`
					IsToggled   bool `json:"isToggled"`
					IsDisabled  bool `json:"isDisabled"`
					DefaultIcon struct {
						IconType string `json:"iconType"`
					} `json:"defaultIcon"`
					ToggledIcon struct {
						IconType string `json:"iconType"`
					} `json:"toggledIcon"`
					TrackingParams string `json:"trackingParams"`
					DefaultTooltip string `json:"defaultTooltip"`
					ToggledTooltip string `json:"toggledTooltip"`
					ToggledStyle   struct {
						StyleType string `json:"styleType"`
					} `json:"toggledStyle"`
					DefaultNavigationEndpoint struct {
						ClickTrackingParams string `json:"clickTrackingParams"`
						CommandMetadata     struct {
							WebCommandMetadata struct {
								IgnoreNavigation bool `json:"ignoreNavigation"`
							} `json:"webCommandMetadata"`
						} `json:"commandMetadata"`
						ModalEndpoint struct {
							Modal struct {
								ModalWithTitleAndButtonRenderer struct {
									Title struct {
										SimpleText string `json:"simpleText"`
									} `json:"title"`
									Content struct {
										SimpleText string `json:"simpleText"`
									} `json:"content"`
									Button struct {
										ButtonRenderer struct {
											Style      string `json:"style"`
											Size       string `json:"size"`
											IsDisabled bool   `json:"isDisabled"`
											Text       struct {
												SimpleText string `json:"simpleText"`
											} `json:"text"`
											NavigationEndpoint struct {
												ClickTrackingParams string `json:"clickTrackingParams"`
												CommandMetadata     struct {
													WebCommandMetadata struct {
														URL         string `json:"url"`
														WebPageType string `json:"webPageType"`
														RootVe      int    `json:"rootVe"`
													} `json:"webCommandMetadata"`
												} `json:"commandMetadata"`
												SignInEndpoint struct {
													NextEndpoint struct {
														ClickTrackingParams string `json:"clickTrackingParams"`
														CommandMetadata     struct {
															WebCommandMetadata struct {
																URL         string `json:"url"`
																WebPageType string `json:"webPageType"`
																RootVe      int    `json:"rootVe"`
																APIURL      string `json:"apiUrl"`
															} `json:"webCommandMetadata"`
														} `json:"commandMetadata"`
														BrowseEndpoint struct {
															BrowseID string `json:"browseId"`
														} `json:"browseEndpoint"`
													} `json:"nextEndpoint"`
													IdamTag string `json:"idamTag"`
												} `json:"signInEndpoint"`
											} `json:"navigationEndpoint"`
											TrackingParams string `json:"trackingParams"`
										} `json:"buttonRenderer"`
									} `json:"button"`
								} `json:"modalWithTitleAndButtonRenderer"`
							} `json:"modal"`
						} `json:"modalEndpoint"`
					} `json:"defaultNavigationEndpoint"`
					AccessibilityData struct {
						AccessibilityData struct {
							Label string `json:"label"`
						} `json:"accessibilityData"`
					} `json:"accessibilityData"`
					ToggledAccessibilityData struct {
						AccessibilityData struct {
							Label string `json:"label"`
						} `json:"accessibilityData"`
					} `json:"toggledAccessibilityData"`
				} `json:"toggleButtonRenderer"`
			} `json:"saveButton"`
			ShareButton struct {
				ButtonRenderer struct {
					Style           string `json:"style"`
					Size            string `json:"size"`
					IsDisabled      bool   `json:"isDisabled"`
					ServiceEndpoint struct {
						ClickTrackingParams string `json:"clickTrackingParams"`
						CommandMetadata     struct {
							WebCommandMetadata struct {
								SendPost bool   `json:"sendPost"`
								APIURL   string `json:"apiUrl"`
							} `json:"webCommandMetadata"`
						} `json:"commandMetadata"`
						ShareEntityServiceEndpoint struct {
							SerializedShareEntity string `json:"serializedShareEntity"`
							Commands              []struct {
								ClickTrackingParams string `json:"clickTrackingParams"`
								OpenPopupAction     struct {
									Popup struct {
										UnifiedSharePanelRenderer struct {
											TrackingParams     string `json:"trackingParams"`
											ShowLoadingSpinner bool   `json:"showLoadingSpinner"`
										} `json:"unifiedSharePanelRenderer"`
									} `json:"popup"`
									PopupType string `json:"popupType"`
									BeReused  bool   `json:"beReused"`
								} `json:"openPopupAction"`
							} `json:"commands"`
						} `json:"shareEntityServiceEndpoint"`
					} `json:"serviceEndpoint"`
					Icon struct {
						IconType string `json:"iconType"`
					} `json:"icon"`
					Tooltip           string `json:"tooltip"`
					TrackingParams    string `json:"trackingParams"`
					AccessibilityData struct {
						AccessibilityData struct {
							Label string `json:"label"`
						} `json:"accessibilityData"`
					} `json:"accessibilityData"`
				} `json:"buttonRenderer"`
			} `json:"shareButton"`
			MoreActionsMenu struct {
				MenuRenderer struct {
					Items []struct {
						MenuNavigationItemRenderer struct {
							Text struct {
								SimpleText string `json:"simpleText"`
							} `json:"text"`
							Icon struct {
								IconType string `json:"iconType"`
							} `json:"icon"`
							NavigationEndpoint struct {
								ClickTrackingParams string `json:"clickTrackingParams"`
								CommandMetadata     struct {
									WebCommandMetadata struct {
										URL         string `json:"url"`
										WebPageType string `json:"webPageType"`
										RootVe      int    `json:"rootVe"`
										APIURL      string `json:"apiUrl"`
									} `json:"webCommandMetadata"`
								} `json:"commandMetadata"`
								BrowseEndpoint struct {
									BrowseID       string `json:"browseId"`
									Params         string `json:"params"`
									Nofollow       bool   `json:"nofollow"`
									NavigationType string `json:"navigationType"`
								} `json:"browseEndpoint"`
							} `json:"navigationEndpoint"`
							TrackingParams string `json:"trackingParams"`
						} `json:"menuNavigationItemRenderer"`
					} `json:"items"`
					TrackingParams string `json:"trackingParams"`
					Accessibility  struct {
						AccessibilityData struct {
							Label string `json:"label"`
						} `json:"accessibilityData"`
					} `json:"accessibility"`
					TargetID string `json:"targetId"`
				} `json:"menuRenderer"`
			} `json:"moreActionsMenu"`
			PlayButton struct {
				ButtonRenderer struct {
					Style      string `json:"style"`
					Size       string `json:"size"`
					IsDisabled bool   `json:"isDisabled"`
					Text       struct {
						SimpleText string `json:"simpleText"`
					} `json:"text"`
					Icon struct {
						IconType string `json:"iconType"`
					} `json:"icon"`
					NavigationEndpoint struct {
						ClickTrackingParams string `json:"clickTrackingParams"`
						CommandMetadata     struct {
							WebCommandMetadata struct {
								URL         string `json:"url"`
								WebPageType string `json:"webPageType"`
								RootVe      int    `json:"rootVe"`
							} `json:"webCommandMetadata"`
						} `json:"commandMetadata"`
						WatchEndpoint struct {
							VideoID        string `json:"videoId"`
							PlaylistID     string `json:"playlistId"`
							PlayerParams   string `json:"playerParams"`
							LoggingContext struct {
								VssLoggingContext struct {
									SerializedContextData string `json:"serializedContextData"`
								} `json:"vssLoggingContext"`
							} `json:"loggingContext"`
							WatchEndpointSupportedOnesieConfig struct {
								HTML5PlaybackOnesieConfig struct {
									CommonConfig struct {
										URL string `json:"url"`
									} `json:"commonConfig"`
								} `json:"html5PlaybackOnesieConfig"`
							} `json:"watchEndpointSupportedOnesieConfig"`
						} `json:"watchEndpoint"`
					} `json:"navigationEndpoint"`
					TrackingParams string `json:"trackingParams"`
				} `json:"buttonRenderer"`
			} `json:"playButton"`
			ShufflePlayButton struct {
				ButtonRenderer struct {
					Style      string `json:"style"`
					Size       string `json:"size"`
					IsDisabled bool   `json:"isDisabled"`
					Text       struct {
						SimpleText string `json:"simpleText"`
					} `json:"text"`
					Icon struct {
						IconType string `json:"iconType"`
					} `json:"icon"`
					NavigationEndpoint struct {
						ClickTrackingParams string `json:"clickTrackingParams"`
						CommandMetadata     struct {
							WebCommandMetadata struct {
								URL         string `json:"url"`
								WebPageType string `json:"webPageType"`
								RootVe      int    `json:"rootVe"`
							} `json:"webCommandMetadata"`
						} `json:"commandMetadata"`
						WatchEndpoint struct {
							VideoID        string `json:"videoId"`
							PlaylistID     string `json:"playlistId"`
							Params         string `json:"params"`
							PlayerParams   string `json:"playerParams"`
							LoggingContext struct {
								VssLoggingContext struct {
									SerializedContextData string `json:"serializedContextData"`
								} `json:"vssLoggingContext"`
							} `json:"loggingContext"`
							WatchEndpointSupportedOnesieConfig struct {
								HTML5PlaybackOnesieConfig struct {
									CommonConfig struct {
										URL string `json:"url"`
									} `json:"commonConfig"`
								} `json:"html5PlaybackOnesieConfig"`
							} `json:"watchEndpointSupportedOnesieConfig"`
						} `json:"watchEndpoint"`
					} `json:"navigationEndpoint"`
					TrackingParams string `json:"trackingParams"`
				} `json:"buttonRenderer"`
			} `json:"shufflePlayButton"`
			OnDescriptionTap struct {
				ClickTrackingParams string `json:"clickTrackingParams"`
				OpenPopupAction     struct {
					Popup struct {
						FancyDismissibleDialogRenderer struct {
							DialogMessage struct {
							} `json:"dialogMessage"`
							Title struct {
								Runs []struct {
									Text string `json:"text"`
								} `json:"runs"`
							} `json:"title"`
							ConfirmLabel struct {
								Runs []struct {
									Text string `json:"text"`
								} `json:"runs"`
							} `json:"confirmLabel"`
							TrackingParams string `json:"trackingParams"`
						} `json:"fancyDismissibleDialogRenderer"`
					} `json:"popup"`
					PopupType string `json:"popupType"`
				} `json:"openPopupAction"`
			} `json:"onDescriptionTap"`
			CinematicContainer struct {
				CinematicContainerRenderer struct {
					BackgroundImageConfig struct {
						Thumbnail struct {
							Thumbnails []struct {
								URL    string `json:"url"`
								Width  int    `json:"width"`
								Height int    `json:"height"`
							} `json:"thumbnails"`
						} `json:"thumbnail"`
					} `json:"backgroundImageConfig"`
					GradientColorConfig []struct {
						LightThemeColor int64 `json:"lightThemeColor"`
						DarkThemeColor  int64 `json:"darkThemeColor"`
						StartLocation   int   `json:"startLocation"`
					} `json:"gradientColorConfig"`
					Config struct {
						LightThemeBackgroundColor int64 `json:"lightThemeBackgroundColor"`
						DarkThemeBackgroundColor  int64 `json:"darkThemeBackgroundColor"`
						ColorSourceSizeMultiplier int   `json:"colorSourceSizeMultiplier"`
						ApplyClientImageBlur      bool  `json:"applyClientImageBlur"`
					} `json:"config"`
				} `json:"cinematicContainerRenderer"`
			} `json:"cinematicContainer"`
			Byline []struct {
				PlaylistBylineRenderer struct {
					Text struct {
						Runs []struct {
							Text string `json:"text"`
						} `json:"runs"`
					} `json:"text"`
				} `json:"playlistBylineRenderer"`
			} `json:"byline"`
			DescriptionTapText struct {
				Runs []struct {
					Text string `json:"text"`
				} `json:"runs"`
			} `json:"descriptionTapText"`
		} `json:"playlistHeaderRenderer"`
	} `json:"header"`
	Alerts []struct {
		AlertWithButtonRenderer struct {
			Type string `json:"type"`
			Text struct {
				SimpleText string `json:"simpleText"`
			} `json:"text"`
			DismissButton struct {
				ButtonRenderer struct {
					Style      string `json:"style"`
					Size       string `json:"size"`
					IsDisabled bool   `json:"isDisabled"`
					Icon       struct {
						IconType string `json:"iconType"`
					} `json:"icon"`
					TrackingParams    string `json:"trackingParams"`
					AccessibilityData struct {
						AccessibilityData struct {
							Label string `json:"label"`
						} `json:"accessibilityData"`
					} `json:"accessibilityData"`
				} `json:"buttonRenderer"`
			} `json:"dismissButton"`
		} `json:"alertWithButtonRenderer"`
	} `json:"alerts"`
	Metadata struct {
		PlaylistMetadataRenderer struct {
			Title                  string `json:"title"`
			AndroidAppindexingLink string `json:"androidAppindexingLink"`
			IosAppindexingLink     string `json:"iosAppindexingLink"`
		} `json:"playlistMetadataRenderer"`
	} `json:"metadata"`
	TrackingParams string `json:"trackingParams"`
	Topbar         struct {
		DesktopTopbarRenderer struct {
			Logo struct {
				TopbarLogoRenderer struct {
					IconImage struct {
						IconType string `json:"iconType"`
					} `json:"iconImage"`
					TooltipText struct {
						Runs []struct {
							Text string `json:"text"`
						} `json:"runs"`
					} `json:"tooltipText"`
					Endpoint struct {
						ClickTrackingParams string `json:"clickTrackingParams"`
						CommandMetadata     struct {
							WebCommandMetadata struct {
								URL         string `json:"url"`
								WebPageType string `json:"webPageType"`
								RootVe      int    `json:"rootVe"`
								APIURL      string `json:"apiUrl"`
							} `json:"webCommandMetadata"`
						} `json:"commandMetadata"`
						BrowseEndpoint struct {
							BrowseID string `json:"browseId"`
						} `json:"browseEndpoint"`
					} `json:"endpoint"`
					TrackingParams    string `json:"trackingParams"`
					OverrideEntityKey string `json:"overrideEntityKey"`
				} `json:"topbarLogoRenderer"`
			} `json:"logo"`
			Searchbox struct {
				FusionSearchboxRenderer struct {
					Icon struct {
						IconType string `json:"iconType"`
					} `json:"icon"`
					PlaceholderText struct {
						Runs []struct {
							Text string `json:"text"`
						} `json:"runs"`
					} `json:"placeholderText"`
					Config struct {
						WebSearchboxConfig struct {
							RequestLanguage     string `json:"requestLanguage"`
							RequestDomain       string `json:"requestDomain"`
							HasOnscreenKeyboard bool   `json:"hasOnscreenKeyboard"`
							FocusSearchbox      bool   `json:"focusSearchbox"`
						} `json:"webSearchboxConfig"`
					} `json:"config"`
					TrackingParams string `json:"trackingParams"`
					SearchEndpoint struct {
						ClickTrackingParams string `json:"clickTrackingParams"`
						CommandMetadata     struct {
							WebCommandMetadata struct {
								URL         string `json:"url"`
								WebPageType string `json:"webPageType"`
								RootVe      int    `json:"rootVe"`
							} `json:"webCommandMetadata"`
						} `json:"commandMetadata"`
						SearchEndpoint struct {
							Query string `json:"query"`
						} `json:"searchEndpoint"`
					} `json:"searchEndpoint"`
					ClearButton struct {
						ButtonRenderer struct {
							Style      string `json:"style"`
							Size       string `json:"size"`
							IsDisabled bool   `json:"isDisabled"`
							Icon       struct {
								IconType string `json:"iconType"`
							} `json:"icon"`
							TrackingParams    string `json:"trackingParams"`
							AccessibilityData struct {
								AccessibilityData struct {
									Label string `json:"label"`
								} `json:"accessibilityData"`
							} `json:"accessibilityData"`
						} `json:"buttonRenderer"`
					} `json:"clearButton"`
				} `json:"fusionSearchboxRenderer"`
			} `json:"searchbox"`
			TrackingParams string `json:"trackingParams"`
			CountryCode    string `json:"countryCode"`
			TopbarButtons  []struct {
				TopbarMenuButtonRenderer struct {
					Icon struct {
						IconType string `json:"iconType"`
					} `json:"icon"`
					MenuRequest struct {
						ClickTrackingParams string `json:"clickTrackingParams"`
						CommandMetadata     struct {
							WebCommandMetadata struct {
								SendPost bool   `json:"sendPost"`
								APIURL   string `json:"apiUrl"`
							} `json:"webCommandMetadata"`
						} `json:"commandMetadata"`
						SignalServiceEndpoint struct {
							Signal  string `json:"signal"`
							Actions []struct {
								ClickTrackingParams string `json:"clickTrackingParams"`
								OpenPopupAction     struct {
									Popup struct {
										MultiPageMenuRenderer struct {
											TrackingParams     string `json:"trackingParams"`
											Style              string `json:"style"`
											ShowLoadingSpinner bool   `json:"showLoadingSpinner"`
										} `json:"multiPageMenuRenderer"`
									} `json:"popup"`
									PopupType string `json:"popupType"`
									BeReused  bool   `json:"beReused"`
								} `json:"openPopupAction"`
							} `json:"actions"`
						} `json:"signalServiceEndpoint"`
					} `json:"menuRequest"`
					TrackingParams string `json:"trackingParams"`
					Accessibility  struct {
						AccessibilityData struct {
							Label string `json:"label"`
						} `json:"accessibilityData"`
					} `json:"accessibility"`
					Tooltip string `json:"tooltip"`
					Style   string `json:"style"`
				} `json:"topbarMenuButtonRenderer,omitempty"`
				ButtonRenderer struct {
					Style string `json:"style"`
					Size  string `json:"size"`
					Text  struct {
						Runs []struct {
							Text string `json:"text"`
						} `json:"runs"`
					} `json:"text"`
					Icon struct {
						IconType string `json:"iconType"`
					} `json:"icon"`
					NavigationEndpoint struct {
						ClickTrackingParams string `json:"clickTrackingParams"`
						CommandMetadata     struct {
							WebCommandMetadata struct {
								URL         string `json:"url"`
								WebPageType string `json:"webPageType"`
								RootVe      int    `json:"rootVe"`
							} `json:"webCommandMetadata"`
						} `json:"commandMetadata"`
						SignInEndpoint struct {
							IdamTag string `json:"idamTag"`
						} `json:"signInEndpoint"`
					} `json:"navigationEndpoint"`
					TrackingParams string `json:"trackingParams"`
					TargetID       string `json:"targetId"`
				} `json:"buttonRenderer,omitempty"`
			} `json:"topbarButtons"`
			HotkeyDialog struct {
				HotkeyDialogRenderer struct {
					Title struct {
						Runs []struct {
							Text string `json:"text"`
						} `json:"runs"`
					} `json:"title"`
					Sections []struct {
						HotkeyDialogSectionRenderer struct {
							Title struct {
								Runs []struct {
									Text string `json:"text"`
								} `json:"runs"`
							} `json:"title"`
							Options []struct {
								HotkeyDialogSectionOptionRenderer struct {
									Label struct {
										Runs []struct {
											Text string `json:"text"`
										} `json:"runs"`
									} `json:"label"`
									Hotkey string `json:"hotkey"`
								} `json:"hotkeyDialogSectionOptionRenderer,omitempty"`
							} `json:"options"`
						} `json:"hotkeyDialogSectionRenderer"`
					} `json:"sections"`
					DismissButton struct {
						ButtonRenderer struct {
							Style      string `json:"style"`
							Size       string `json:"size"`
							IsDisabled bool   `json:"isDisabled"`
							Text       struct {
								Runs []struct {
									Text string `json:"text"`
								} `json:"runs"`
							} `json:"text"`
							TrackingParams string `json:"trackingParams"`
						} `json:"buttonRenderer"`
					} `json:"dismissButton"`
					TrackingParams string `json:"trackingParams"`
				} `json:"hotkeyDialogRenderer"`
			} `json:"hotkeyDialog"`
			BackButton struct {
				ButtonRenderer struct {
					TrackingParams string `json:"trackingParams"`
					Command        struct {
						ClickTrackingParams string `json:"clickTrackingParams"`
						CommandMetadata     struct {
							WebCommandMetadata struct {
								SendPost bool `json:"sendPost"`
							} `json:"webCommandMetadata"`
						} `json:"commandMetadata"`
						SignalServiceEndpoint struct {
							Signal  string `json:"signal"`
							Actions []struct {
								ClickTrackingParams string `json:"clickTrackingParams"`
								SignalAction        struct {
									Signal string `json:"signal"`
								} `json:"signalAction"`
							} `json:"actions"`
						} `json:"signalServiceEndpoint"`
					} `json:"command"`
				} `json:"buttonRenderer"`
			} `json:"backButton"`
			ForwardButton struct {
				ButtonRenderer struct {
					TrackingParams string `json:"trackingParams"`
					Command        struct {
						ClickTrackingParams string `json:"clickTrackingParams"`
						CommandMetadata     struct {
							WebCommandMetadata struct {
								SendPost bool `json:"sendPost"`
							} `json:"webCommandMetadata"`
						} `json:"commandMetadata"`
						SignalServiceEndpoint struct {
							Signal  string `json:"signal"`
							Actions []struct {
								ClickTrackingParams string `json:"clickTrackingParams"`
								SignalAction        struct {
									Signal string `json:"signal"`
								} `json:"signalAction"`
							} `json:"actions"`
						} `json:"signalServiceEndpoint"`
					} `json:"command"`
				} `json:"buttonRenderer"`
			} `json:"forwardButton"`
			A11YSkipNavigationButton struct {
				ButtonRenderer struct {
					Style      string `json:"style"`
					Size       string `json:"size"`
					IsDisabled bool   `json:"isDisabled"`
					Text       struct {
						Runs []struct {
							Text string `json:"text"`
						} `json:"runs"`
					} `json:"text"`
					TrackingParams string `json:"trackingParams"`
					Command        struct {
						ClickTrackingParams string `json:"clickTrackingParams"`
						CommandMetadata     struct {
							WebCommandMetadata struct {
								SendPost bool `json:"sendPost"`
							} `json:"webCommandMetadata"`
						} `json:"commandMetadata"`
						SignalServiceEndpoint struct {
							Signal  string `json:"signal"`
							Actions []struct {
								ClickTrackingParams string `json:"clickTrackingParams"`
								SignalAction        struct {
									Signal string `json:"signal"`
								} `json:"signalAction"`
							} `json:"actions"`
						} `json:"signalServiceEndpoint"`
					} `json:"command"`
				} `json:"buttonRenderer"`
			} `json:"a11ySkipNavigationButton"`
			VoiceSearchButton struct {
				ButtonRenderer struct {
					Style           string `json:"style"`
					Size            string `json:"size"`
					IsDisabled      bool   `json:"isDisabled"`
					ServiceEndpoint struct {
						ClickTrackingParams string `json:"clickTrackingParams"`
						CommandMetadata     struct {
							WebCommandMetadata struct {
								SendPost bool `json:"sendPost"`
							} `json:"webCommandMetadata"`
						} `json:"commandMetadata"`
						SignalServiceEndpoint struct {
							Signal  string `json:"signal"`
							Actions []struct {
								ClickTrackingParams string `json:"clickTrackingParams"`
								OpenPopupAction     struct {
									Popup struct {
										VoiceSearchDialogRenderer struct {
											PlaceholderHeader struct {
												Runs []struct {
													Text string `json:"text"`
												} `json:"runs"`
											} `json:"placeholderHeader"`
											PromptHeader struct {
												Runs []struct {
													Text string `json:"text"`
												} `json:"runs"`
											} `json:"promptHeader"`
											ExampleQuery1 struct {
												Runs []struct {
													Text string `json:"text"`
												} `json:"runs"`
											} `json:"exampleQuery1"`
											ExampleQuery2 struct {
												Runs []struct {
													Text string `json:"text"`
												} `json:"runs"`
											} `json:"exampleQuery2"`
											PromptMicrophoneLabel struct {
												Runs []struct {
													Text string `json:"text"`
												} `json:"runs"`
											} `json:"promptMicrophoneLabel"`
											LoadingHeader struct {
												Runs []struct {
													Text string `json:"text"`
												} `json:"runs"`
											} `json:"loadingHeader"`
											ConnectionErrorHeader struct {
												Runs []struct {
													Text string `json:"text"`
												} `json:"runs"`
											} `json:"connectionErrorHeader"`
											ConnectionErrorMicrophoneLabel struct {
												Runs []struct {
													Text string `json:"text"`
												} `json:"runs"`
											} `json:"connectionErrorMicrophoneLabel"`
											PermissionsHeader struct {
												Runs []struct {
													Text string `json:"text"`
												} `json:"runs"`
											} `json:"permissionsHeader"`
											PermissionsSubtext struct {
												Runs []struct {
													Text string `json:"text"`
												} `json:"runs"`
											} `json:"permissionsSubtext"`
											DisabledHeader struct {
												Runs []struct {
													Text string `json:"text"`
												} `json:"runs"`
											} `json:"disabledHeader"`
											DisabledSubtext struct {
												Runs []struct {
													Text string `json:"text"`
												} `json:"runs"`
											} `json:"disabledSubtext"`
											MicrophoneButtonAriaLabel struct {
												Runs []struct {
													Text string `json:"text"`
												} `json:"runs"`
											} `json:"microphoneButtonAriaLabel"`
											ExitButton struct {
												ButtonRenderer struct {
													Style      string `json:"style"`
													Size       string `json:"size"`
													IsDisabled bool   `json:"isDisabled"`
													Icon       struct {
														IconType string `json:"iconType"`
													} `json:"icon"`
													TrackingParams    string `json:"trackingParams"`
													AccessibilityData struct {
														AccessibilityData struct {
															Label string `json:"label"`
														} `json:"accessibilityData"`
													} `json:"accessibilityData"`
												} `json:"buttonRenderer"`
											} `json:"exitButton"`
											TrackingParams            string `json:"trackingParams"`
											MicrophoneOffPromptHeader struct {
												Runs []struct {
													Text string `json:"text"`
												} `json:"runs"`
											} `json:"microphoneOffPromptHeader"`
										} `json:"voiceSearchDialogRenderer"`
									} `json:"popup"`
									PopupType string `json:"popupType"`
								} `json:"openPopupAction"`
							} `json:"actions"`
						} `json:"signalServiceEndpoint"`
					} `json:"serviceEndpoint"`
					Icon struct {
						IconType string `json:"iconType"`
					} `json:"icon"`
					Tooltip           string `json:"tooltip"`
					TrackingParams    string `json:"trackingParams"`
					AccessibilityData struct {
						AccessibilityData struct {
							Label string `json:"label"`
						} `json:"accessibilityData"`
					} `json:"accessibilityData"`
				} `json:"buttonRenderer"`
			} `json:"voiceSearchButton"`
		} `json:"desktopTopbarRenderer"`
	} `json:"topbar"`
	Microformat struct {
		MicroformatDataRenderer struct {
			URLCanonical string `json:"urlCanonical"`
			Title        string `json:"title"`
			Description  string `json:"description"`
			Thumbnail    struct {
				Thumbnails []struct {
					URL    string `json:"url"`
					Width  int    `json:"width"`
					Height int    `json:"height"`
				} `json:"thumbnails"`
			} `json:"thumbnail"`
			SiteName           string `json:"siteName"`
			AppName            string `json:"appName"`
			AndroidPackage     string `json:"androidPackage"`
			IosAppStoreID      string `json:"iosAppStoreId"`
			IosAppArguments    string `json:"iosAppArguments"`
			OgType             string `json:"ogType"`
			URLApplinksWeb     string `json:"urlApplinksWeb"`
			URLApplinksIos     string `json:"urlApplinksIos"`
			URLApplinksAndroid string `json:"urlApplinksAndroid"`
			URLTwitterIos      string `json:"urlTwitterIos"`
			URLTwitterAndroid  string `json:"urlTwitterAndroid"`
			TwitterCardType    string `json:"twitterCardType"`
			TwitterSiteHandle  string `json:"twitterSiteHandle"`
			SchemaDotOrgType   string `json:"schemaDotOrgType"`
			Noindex            bool   `json:"noindex"`
			Unlisted           bool   `json:"unlisted"`
			LinkAlternates     []struct {
				HrefURL string `json:"hrefUrl"`
			} `json:"linkAlternates"`
		} `json:"microformatDataRenderer"`
	} `json:"microformat"`
	Sidebar struct {
		PlaylistSidebarRenderer struct {
			Items []struct {
				PlaylistSidebarPrimaryInfoRenderer struct {
					ThumbnailRenderer struct {
						PlaylistVideoThumbnailRenderer struct {
							Thumbnail struct {
								Thumbnails []struct {
									URL    string `json:"url"`
									Width  int    `json:"width"`
									Height int    `json:"height"`
								} `json:"thumbnails"`
							} `json:"thumbnail"`
							TrackingParams string `json:"trackingParams"`
						} `json:"playlistVideoThumbnailRenderer"`
					} `json:"thumbnailRenderer"`
					Title struct {
						Runs []struct {
							Text               string `json:"text"`
							NavigationEndpoint struct {
								ClickTrackingParams string `json:"clickTrackingParams"`
								CommandMetadata     struct {
									WebCommandMetadata struct {
										URL         string `json:"url"`
										WebPageType string `json:"webPageType"`
										RootVe      int    `json:"rootVe"`
									} `json:"webCommandMetadata"`
								} `json:"commandMetadata"`
								WatchEndpoint struct {
									VideoID        string `json:"videoId"`
									PlaylistID     string `json:"playlistId"`
									PlayerParams   string `json:"playerParams"`
									LoggingContext struct {
										VssLoggingContext struct {
											SerializedContextData string `json:"serializedContextData"`
										} `json:"vssLoggingContext"`
									} `json:"loggingContext"`
									WatchEndpointSupportedOnesieConfig struct {
										HTML5PlaybackOnesieConfig struct {
											CommonConfig struct {
												URL string `json:"url"`
											} `json:"commonConfig"`
										} `json:"html5PlaybackOnesieConfig"`
									} `json:"watchEndpointSupportedOnesieConfig"`
								} `json:"watchEndpoint"`
							} `json:"navigationEndpoint"`
						} `json:"runs"`
					} `json:"title"`
					Stats []struct {
						Runs []struct {
							Text string `json:"text"`
						} `json:"runs,omitempty"`
						SimpleText string `json:"simpleText,omitempty"`
					} `json:"stats"`
					Menu struct {
						MenuRenderer struct {
							Items []struct {
								MenuNavigationItemRenderer struct {
									Text struct {
										SimpleText string `json:"simpleText"`
									} `json:"text"`
									Icon struct {
										IconType string `json:"iconType"`
									} `json:"icon"`
									NavigationEndpoint struct {
										ClickTrackingParams string `json:"clickTrackingParams"`
										CommandMetadata     struct {
											WebCommandMetadata struct {
												URL         string `json:"url"`
												WebPageType string `json:"webPageType"`
												RootVe      int    `json:"rootVe"`
												APIURL      string `json:"apiUrl"`
											} `json:"webCommandMetadata"`
										} `json:"commandMetadata"`
										BrowseEndpoint struct {
											BrowseID       string `json:"browseId"`
											Params         string `json:"params"`
											Nofollow       bool   `json:"nofollow"`
											NavigationType string `json:"navigationType"`
										} `json:"browseEndpoint"`
									} `json:"navigationEndpoint"`
									TrackingParams string `json:"trackingParams"`
								} `json:"menuNavigationItemRenderer"`
							} `json:"items"`
							TrackingParams  string `json:"trackingParams"`
							TopLevelButtons []struct {
								ToggleButtonRenderer struct {
									Style struct {
										StyleType string `json:"styleType"`
									} `json:"style"`
									Size struct {
										SizeType string `json:"sizeType"`
									} `json:"size"`
									IsToggled   bool `json:"isToggled"`
									IsDisabled  bool `json:"isDisabled"`
									DefaultIcon struct {
										IconType string `json:"iconType"`
									} `json:"defaultIcon"`
									ToggledIcon struct {
										IconType string `json:"iconType"`
									} `json:"toggledIcon"`
									TrackingParams            string `json:"trackingParams"`
									DefaultTooltip            string `json:"defaultTooltip"`
									ToggledTooltip            string `json:"toggledTooltip"`
									DefaultNavigationEndpoint struct {
										ClickTrackingParams string `json:"clickTrackingParams"`
										CommandMetadata     struct {
											WebCommandMetadata struct {
												IgnoreNavigation bool `json:"ignoreNavigation"`
											} `json:"webCommandMetadata"`
										} `json:"commandMetadata"`
										ModalEndpoint struct {
											Modal struct {
												ModalWithTitleAndButtonRenderer struct {
													Title struct {
														SimpleText string `json:"simpleText"`
													} `json:"title"`
													Content struct {
														SimpleText string `json:"simpleText"`
													} `json:"content"`
													Button struct {
														ButtonRenderer struct {
															Style      string `json:"style"`
															Size       string `json:"size"`
															IsDisabled bool   `json:"isDisabled"`
															Text       struct {
																SimpleText string `json:"simpleText"`
															} `json:"text"`
															NavigationEndpoint struct {
																ClickTrackingParams string `json:"clickTrackingParams"`
																CommandMetadata     struct {
																	WebCommandMetadata struct {
																		URL         string `json:"url"`
																		WebPageType string `json:"webPageType"`
																		RootVe      int    `json:"rootVe"`
																	} `json:"webCommandMetadata"`
																} `json:"commandMetadata"`
																SignInEndpoint struct {
																	NextEndpoint struct {
																		ClickTrackingParams string `json:"clickTrackingParams"`
																		CommandMetadata     struct {
																			WebCommandMetadata struct {
																				URL         string `json:"url"`
																				WebPageType string `json:"webPageType"`
																				RootVe      int    `json:"rootVe"`
																				APIURL      string `json:"apiUrl"`
																			} `json:"webCommandMetadata"`
																		} `json:"commandMetadata"`
																		BrowseEndpoint struct {
																			BrowseID string `json:"browseId"`
																		} `json:"browseEndpoint"`
																	} `json:"nextEndpoint"`
																	IdamTag string `json:"idamTag"`
																} `json:"signInEndpoint"`
															} `json:"navigationEndpoint"`
															TrackingParams string `json:"trackingParams"`
														} `json:"buttonRenderer"`
													} `json:"button"`
												} `json:"modalWithTitleAndButtonRenderer"`
											} `json:"modal"`
										} `json:"modalEndpoint"`
									} `json:"defaultNavigationEndpoint"`
									AccessibilityData struct {
										AccessibilityData struct {
											Label string `json:"label"`
										} `json:"accessibilityData"`
									} `json:"accessibilityData"`
									ToggledAccessibilityData struct {
										AccessibilityData struct {
											Label string `json:"label"`
										} `json:"accessibilityData"`
									} `json:"toggledAccessibilityData"`
								} `json:"toggleButtonRenderer,omitempty"`
								ButtonRenderer struct {
									Style      string `json:"style"`
									Size       string `json:"size"`
									IsDisabled bool   `json:"isDisabled"`
									Icon       struct {
										IconType string `json:"iconType"`
									} `json:"icon"`
									NavigationEndpoint struct {
										ClickTrackingParams string `json:"clickTrackingParams"`
										CommandMetadata     struct {
											WebCommandMetadata struct {
												URL         string `json:"url"`
												WebPageType string `json:"webPageType"`
												RootVe      int    `json:"rootVe"`
											} `json:"webCommandMetadata"`
										} `json:"commandMetadata"`
										WatchEndpoint struct {
											VideoID        string `json:"videoId"`
											PlaylistID     string `json:"playlistId"`
											Params         string `json:"params"`
											PlayerParams   string `json:"playerParams"`
											LoggingContext struct {
												VssLoggingContext struct {
													SerializedContextData string `json:"serializedContextData"`
												} `json:"vssLoggingContext"`
											} `json:"loggingContext"`
											WatchEndpointSupportedOnesieConfig struct {
												HTML5PlaybackOnesieConfig struct {
													CommonConfig struct {
														URL string `json:"url"`
													} `json:"commonConfig"`
												} `json:"html5PlaybackOnesieConfig"`
											} `json:"watchEndpointSupportedOnesieConfig"`
										} `json:"watchEndpoint"`
									} `json:"navigationEndpoint"`
									Accessibility struct {
										Label string `json:"label"`
									} `json:"accessibility"`
									Tooltip        string `json:"tooltip"`
									TrackingParams string `json:"trackingParams"`
								} `json:"buttonRenderer,omitempty"`
							} `json:"topLevelButtons"`
							Accessibility struct {
								AccessibilityData struct {
									Label string `json:"label"`
								} `json:"accessibilityData"`
							} `json:"accessibility"`
							TargetID string `json:"targetId"`
						} `json:"menuRenderer"`
					} `json:"menu"`
					ThumbnailOverlays []struct {
						ThumbnailOverlaySidePanelRenderer struct {
							Text struct {
								SimpleText string `json:"simpleText"`
							} `json:"text"`
							Icon struct {
								IconType string `json:"iconType"`
							} `json:"icon"`
						} `json:"thumbnailOverlaySidePanelRenderer"`
					} `json:"thumbnailOverlays"`
					NavigationEndpoint struct {
						ClickTrackingParams string `json:"clickTrackingParams"`
						CommandMetadata     struct {
							WebCommandMetadata struct {
								URL         string `json:"url"`
								WebPageType string `json:"webPageType"`
								RootVe      int    `json:"rootVe"`
							} `json:"webCommandMetadata"`
						} `json:"commandMetadata"`
						WatchEndpoint struct {
							VideoID        string `json:"videoId"`
							PlaylistID     string `json:"playlistId"`
							PlayerParams   string `json:"playerParams"`
							LoggingContext struct {
								VssLoggingContext struct {
									SerializedContextData string `json:"serializedContextData"`
								} `json:"vssLoggingContext"`
							} `json:"loggingContext"`
							WatchEndpointSupportedOnesieConfig struct {
								HTML5PlaybackOnesieConfig struct {
									CommonConfig struct {
										URL string `json:"url"`
									} `json:"commonConfig"`
								} `json:"html5PlaybackOnesieConfig"`
							} `json:"watchEndpointSupportedOnesieConfig"`
						} `json:"watchEndpoint"`
					} `json:"navigationEndpoint"`
					Description struct {
					} `json:"description"`
					ShowMoreText struct {
						Runs []struct {
							Text string `json:"text"`
						} `json:"runs"`
					} `json:"showMoreText"`
				} `json:"playlistSidebarPrimaryInfoRenderer,omitempty"`
				PlaylistSidebarSecondaryInfoRenderer struct {
					VideoOwner struct {
						VideoOwnerRenderer struct {
							Thumbnail struct {
								Thumbnails []struct {
									URL    string `json:"url"`
									Width  int    `json:"width"`
									Height int    `json:"height"`
								} `json:"thumbnails"`
							} `json:"thumbnail"`
							Title struct {
								Runs []struct {
									Text               string `json:"text"`
									NavigationEndpoint struct {
										ClickTrackingParams string `json:"clickTrackingParams"`
										CommandMetadata     struct {
											WebCommandMetadata struct {
												URL         string `json:"url"`
												WebPageType string `json:"webPageType"`
												RootVe      int    `json:"rootVe"`
												APIURL      string `json:"apiUrl"`
											} `json:"webCommandMetadata"`
										} `json:"commandMetadata"`
										BrowseEndpoint struct {
											BrowseID         string `json:"browseId"`
											CanonicalBaseURL string `json:"canonicalBaseUrl"`
										} `json:"browseEndpoint"`
									} `json:"navigationEndpoint"`
								} `json:"runs"`
							} `json:"title"`
							NavigationEndpoint struct {
								ClickTrackingParams string `json:"clickTrackingParams"`
								CommandMetadata     struct {
									WebCommandMetadata struct {
										URL         string `json:"url"`
										WebPageType string `json:"webPageType"`
										RootVe      int    `json:"rootVe"`
										APIURL      string `json:"apiUrl"`
									} `json:"webCommandMetadata"`
								} `json:"commandMetadata"`
								BrowseEndpoint struct {
									BrowseID         string `json:"browseId"`
									CanonicalBaseURL string `json:"canonicalBaseUrl"`
								} `json:"browseEndpoint"`
							} `json:"navigationEndpoint"`
							TrackingParams string `json:"trackingParams"`
						} `json:"videoOwnerRenderer"`
					} `json:"videoOwner"`
					Button struct {
						ButtonRenderer struct {
							Style      string `json:"style"`
							Size       string `json:"size"`
							IsDisabled bool   `json:"isDisabled"`
							Text       struct {
								Runs []struct {
									Text string `json:"text"`
								} `json:"runs"`
							} `json:"text"`
							NavigationEndpoint struct {
								ClickTrackingParams string `json:"clickTrackingParams"`
								CommandMetadata     struct {
									WebCommandMetadata struct {
										IgnoreNavigation bool `json:"ignoreNavigation"`
									} `json:"webCommandMetadata"`
								} `json:"commandMetadata"`
								ModalEndpoint struct {
									Modal struct {
										ModalWithTitleAndButtonRenderer struct {
											Title struct {
												SimpleText string `json:"simpleText"`
											} `json:"title"`
											Content struct {
												SimpleText string `json:"simpleText"`
											} `json:"content"`
											Button struct {
												ButtonRenderer struct {
													Style      string `json:"style"`
													Size       string `json:"size"`
													IsDisabled bool   `json:"isDisabled"`
													Text       struct {
														SimpleText string `json:"simpleText"`
													} `json:"text"`
													NavigationEndpoint struct {
														ClickTrackingParams string `json:"clickTrackingParams"`
														CommandMetadata     struct {
															WebCommandMetadata struct {
																URL         string `json:"url"`
																WebPageType string `json:"webPageType"`
																RootVe      int    `json:"rootVe"`
															} `json:"webCommandMetadata"`
														} `json:"commandMetadata"`
														SignInEndpoint struct {
															NextEndpoint struct {
																ClickTrackingParams string `json:"clickTrackingParams"`
																CommandMetadata     struct {
																	WebCommandMetadata struct {
																		URL         string `json:"url"`
																		WebPageType string `json:"webPageType"`
																		RootVe      int    `json:"rootVe"`
																		APIURL      string `json:"apiUrl"`
																	} `json:"webCommandMetadata"`
																} `json:"commandMetadata"`
																BrowseEndpoint struct {
																	BrowseID string `json:"browseId"`
																} `json:"browseEndpoint"`
															} `json:"nextEndpoint"`
															ContinueAction string `json:"continueAction"`
															IdamTag        string `json:"idamTag"`
														} `json:"signInEndpoint"`
													} `json:"navigationEndpoint"`
													TrackingParams string `json:"trackingParams"`
												} `json:"buttonRenderer"`
											} `json:"button"`
										} `json:"modalWithTitleAndButtonRenderer"`
									} `json:"modal"`
								} `json:"modalEndpoint"`
							} `json:"navigationEndpoint"`
							TrackingParams string `json:"trackingParams"`
						} `json:"buttonRenderer"`
					} `json:"button"`
				} `json:"playlistSidebarSecondaryInfoRenderer,omitempty"`
			} `json:"items"`
			TrackingParams string `json:"trackingParams"`
		} `json:"playlistSidebarRenderer"`
	} `json:"sidebar"`
}
