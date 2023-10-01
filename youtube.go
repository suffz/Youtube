package Youtube

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/faiface/beep"
	"github.com/faiface/beep/mp3"
	"github.com/faiface/beep/speaker"
	"github.com/google/uuid"
)

func VideoTitle(ID string) string {
	var playerResponsePattern = regexp.MustCompile(`var ytInitialPlayerResponse\s*=\s*(\{.+?\});`)
	if resp, err := http.Get("https://www.youtube.com/watch?v=" + ID + "&bpctr=9999999999&has_verified=1"); err == nil {
		body, _ := io.ReadAll(resp.Body)
		initialPlayerResponse := playerResponsePattern.FindSubmatch(body)
		if initialPlayerResponse == nil || len(initialPlayerResponse) < 2 {
			return ""
		}
		var getOpts YT
		json.Unmarshal(initialPlayerResponse[1], &getOpts)
		return getOpts.VideoDetails.Title
	}
	return ""
}

// Video Quality doesnt download all of said videos from the quality chosen, it instead takes what ever is found and prioritizes it once found
// for example {420p, 1080p} 420 comes first so it finds that resolution.
func Video(U Youtube, VideoQuality []string, AudioQuality []string, VideoAndAudio bool) (audio YTRequest, video YTRequest) {
	ID := U.ID
	var playerResponsePattern = regexp.MustCompile(`var ytInitialPlayerResponse\s*=\s*(\{.+?\});`)
	if resp, err := http.Get("https://www.youtube.com/watch?v=" + ID + "&bpctr=9999999999&has_verified=1"); err == nil {
		body, _ := io.ReadAll(resp.Body)
		initialPlayerResponse := playerResponsePattern.FindSubmatch(body)
		if initialPlayerResponse == nil || len(initialPlayerResponse) < 2 {
			return YTRequest{}, YTRequest{}
		}
		var getOpts YT
		json.Unmarshal(initialPlayerResponse[1], &getOpts)
		var Options_Mime []Priority
		var OMime Priority
		var found_prio bool
		if len(VideoQuality) != 0 {
			for _, T := range getOpts.StreamingData.AdaptiveFormats {
				if inside(T.QualityLabel, VideoQuality) {
					if strings.Contains(T.QualityLabel, VideoQuality[0]) {
						Options_Mime = append(Options_Mime, Priority{Top: true, Options_Mime: T})
						continue
					}
					Options_Mime = append(Options_Mime, Priority{Options_Mime: T})
				}
			}
			for _, t := range Options_Mime {
				if t.Top {
					OMime = t
					found_prio = true
					break
				}
			}
			if !found_prio {
				OMime = Options_Mime[0]
			}
			switch true {
			case found_prio && len(VideoQuality) != 0 && len(AudioQuality) == 0:
				return YTRequest{}, dlFormatDlInfo(ID, getOpts, OMime.Options_Mime, resp.Header.Get("Content-Length"))
			case !found_prio && !VideoAndAudio:
				return YTRequest{}, dlFormatDlInfo(ID, getOpts, OMime.Options_Mime, resp.Header.Get("Content-Length"))
			case VideoAndAudio:
				for _, T := range getOpts.StreamingData.AdaptiveFormats {
					switch true {
					case inside(T.AudioQuality, AudioQuality) && T.QualityLabel == "":
						return dlFormatDlInfo(ID, getOpts, T, resp.Header.Get("Content-Length")), dlFormatDlInfo(ID, getOpts, OMime.Options_Mime, resp.Header.Get("Content-Length"))
					}
				}
			}
		}

		// Use Audio Only

		for _, T := range getOpts.StreamingData.AdaptiveFormats {
			switch true {
			case len(VideoQuality) == 0 && len(AudioQuality) != 0 && inside(T.AudioQuality, AudioQuality) && T.QualityLabel == "":
				return dlFormatDlInfo(ID, getOpts, T, resp.Header.Get("Content-Length")), YTRequest{}
			}
		}

	}
	return YTRequest{}, YTRequest{}
}

// Video Quality doesnt download all of said videos from the quality chosen, it instead takes what ever is found and prioritizes it once found
// for example {420p, 1080p} 420 comes first so it finds that resolution.
func VideoAndAudioDownload(ID string, VideoQuality []string, AudioQuality []string) (audio []byte, video []byte, Info YTRequest) {

	var (
		wg                   sync.WaitGroup
		audio_b              []byte
		video_b              []byte
		vid_audio, vid_video = Video(Youtube{
			ID: ID,
		}, VideoQuality, AudioQuality, true)
	)

	if vid_audio.Config.ID == "" || vid_video.Config.ID == "" {
		return nil, nil, YTRequest{}
	}

	wg.Add(2)

	go func() {
		defer wg.Done()
		video_b = vid_video.Download()
	}()
	go func() {
		defer wg.Done()
		audio_b = vid_audio.Download()
	}()

	wg.Wait()

	return audio_b, video_b, vid_audio
}

func AudioMust(audio YTRequest, video YTRequest) *YTRequest {
	return &audio
}

func VideoMust(audio YTRequest, video YTRequest) *YTRequest {
	return &video
}

func FfmpegVideoAndAudio(audio []byte, video []byte, remove_when_complete bool) []byte {
	path := strings.ReplaceAll(uuid.NewString(), "-", "")

	a_f := "audio_f_" + path + ".mp4"
	v_f := "video_f_" + path + ".mp4"

	file, _ := os.Create(a_f)
	file_, _ := os.Create(v_f)
	file.Write(audio)
	file_.Write(video)

	file.Close()
	file_.Close()

	dest := "va_dest_" + path + ".mp4"

	ffmpegVersionCmd := exec.Command("ffmpeg", "-y",
		"-i", v_f,
		"-i", a_f,
		"-c", "copy",
		"-shortest",
		dest,
		"-loglevel", "warning",
	)

	ffmpegVersionCmd.Run()
	if body, err := os.ReadFile(dest); err == nil {
		os.Remove(a_f)
		os.Remove(v_f)
		if remove_when_complete {
			os.Remove(dest)
		}
		return body
	}
	return nil
}

func inside(q string, all []string) bool {
	for _, a := range all {
		if strings.Contains(a, q) {
			return true
		}
	}
	return false
}

func dlFormatDlInfo(ID string, getOpts YT, T AdaptiveFormats, header string) YTRequest {
	ms, _ := strconv.Atoi(T.ApproxDurationMs)
	DL := Youtube{
		ID:      ID,
		FullURL: "https://www.youtube.com/watch?v=" + ID,
		Title:   getOpts.VideoDetails.Title,
		Length:  T.ContentLength,
		Index:   getOpts.VideoDetails.ShortDescription,
		MS:      ms,
		F:       T,
	}
	if T.ContentLength == "" || T.ContentLength == "0" {
		T.ContentLength = header
	}
	return DL.getReq(T.URL, T.Sig, T.ContentLength)
}

func Playlist(url string) (IDs []Youtube) {
	req, _ := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	rr, _ := http.DefaultClient.Do(req)
	aa, _ := io.ReadAll(rr.Body)
	var DD YTPageConfig
	json.Unmarshal([]byte(strings.Split(strings.Split(string(aa), `var ytInitialData =`)[1], `;</script>`)[0]), &DD)
	for _, data := range DD.Contents.TwoColumnBrowseResultsRenderer.Tabs {
		for _, yt := range data.TabRenderer.Content.SectionListRenderer.Contents {
			for _, pagedata := range yt.ItemSectionRenderer.Contents {
				for _, data := range pagedata.PlaylistVideoListRenderer.Contents {
					if data.PlaylistVideoRenderer.IsPlayable && data.PlaylistVideoRenderer.VideoID != "" {
						IDs = append(IDs, Youtube{
							ID:           data.PlaylistVideoRenderer.VideoID,
							Title:        getTitle(data.PlaylistVideoRenderer.Title.Runs),
							FullURL:      "https://www.youtube.com/watch?v=" + data.PlaylistVideoRenderer.VideoID,
							Continuation: data.ContinuationItemRenderer.ContinuationEndpoint.ContinuationCommand.Token,
							Length:       data.PlaylistVideoRenderer.LengthText.SimpleText,
							Index:        data.PlaylistVideoRenderer.Index.SimpleText,
							Info:         getTitle(data.PlaylistVideoRenderer.VideoInfo.Runs),
						})
					} else {
						if data.ContinuationItemRenderer.ContinuationEndpoint.ContinuationCommand.Token != "" {
							IDs[len(IDs)-1].Continuation = data.ContinuationItemRenderer.ContinuationEndpoint.ContinuationCommand.Token
						}
					}
				}
			}
		}
	}
	return
}

// This uses the mp4 []byte from Download() to return a usable mp3 []byte!
func Ffmpeg(buf []byte) (beep.StreamSeekCloser, beep.Format) {

	ffmp, _ := checkFFM()
	cmd := exec.Command(ffmp, "-y",
		"-i", "pipe:0",
		"-map_metadata", "-1",
		"-c:a", "libmp3lame",
		"-fps_mode", "2",
		"-b:a", "128k",
		"-f", "mp3",
		"-vn", "pipe:1",
	)

	resultBuffer := bytes.NewBuffer(make([]byte, 5*1024*1024)) // pre allocate 5MiB buffer
	cmd.Stdout = resultBuffer                                  // stdout result will be written here

	stdin, err := cmd.StdinPipe() // Open stdin pipe
	check(err)

	err = cmd.Start() // Start a process on another goroutine
	check(err)

	_, err = stdin.Write(buf) // pump audio data to stdin pipe
	check(err)

	err = stdin.Close() // close the stdin, or ffmpeg will wait forever
	check(err)

	err = cmd.Wait() // wait until ffmpeg finish
	check(err)

	streamer, f, err := mp3.Decode(io.NopCloser(bytes.NewBuffer(resultBuffer.Bytes())))
	check(err)

	return streamer, f
}
func check(err error) {
	if err != nil {
		panic(err)
	}
}
func dlPeice(re *http.Request, ctx context.Context, start, end int) []byte {
	q := re.URL.Query()
	q.Set("range", fmt.Sprintf("%d-%d", start, end))
	re.URL.RawQuery = q.Encode()
	re.Header.Add("Accept", "*/*")
	re.Header.Set("Origin", "https://youtube.com")
	re.Header.Set("Sec-Fetch-Mode", "navigate")
	re.Header.Set("X-Youtube-Client-Name", "3")
	re.Header.Set("X-Youtube-Client-Version", "17.31.35")
	re.Header.Set("User-Agent", "com.google.android.youtube/17.31.35 (Linux; U; Android 11) gzip")
	re.AddCookie(&http.Cookie{
		Name:   "CONSENT",
		Value:  "YES+cb.20210328-17-p0.en+FX+" + strconv.Itoa(rand.Intn(899)+100),
		Path:   "/",
		Domain: ".youtube.com",
	})
	if r, err := http.DefaultClient.Do(re); err == nil {
		data, _ := io.ReadAll(r.Body)
		r.Body.Close()
		return data
	}
	return nil
}

func (DL *YTRequest) Download() []byte {
	val, _ := strconv.Atoi(DL.ContentLength)
	return DL.dlChunked(val)
}

func (DL *YTRequest) dlChunked(val int) []byte {

	r, w := io.Pipe()
	cancelCtx, cancel := context.WithCancel(context.TODO())
	abort := func(err error) {
		w.CloseWithError(err)
		cancel()
	}
	chunks := getChunks(int64(val), Size1Mb)
	var ctx = context.Background()

	if req, err := http.NewRequestWithContext(ctx, "GET", DL.URL, nil); err == nil {
		var wg sync.WaitGroup
		for _, ch := range chunks {
			wg.Add(1)
			go func(chunk chunk) {
				defer wg.Done()
				data := dlPeice(req.Clone(ctx), ctx, int(chunk.start), int(chunk.end))
				expected := int(chunk.end-chunk.start) + 1
				n := len(data)
				if n == expected {
					chunk.data <- data
				}
				close(chunk.data)
			}(ch)
		}
		go func() {
			for i := 0; i < len(chunks); i++ {
				select {
				case <-cancelCtx.Done():
					abort(context.Canceled)
					return
				case data := <-chunks[i].data:
					_, err := io.Copy(w, bytes.NewBuffer(data))
					if err != nil {
						abort(err)
					}
				}
			}
			w.Close()
		}()
		wg.Wait()
		var buf bytes.Buffer
		mw := io.MultiWriter(&buf)
		io.Copy(mw, r)
		return buf.Bytes()
	}
	return nil
}

func getMaxRoutines(limit int) int {
	routines := 10

	if limit > 0 && routines > limit {
		routines = limit
	}

	return routines
}

type chunk struct {
	start int64
	end   int64
	data  chan []byte
}

func getChunks(totalSize, chunkSize int64) []chunk {
	var chunks []chunk

	for start := int64(0); start < totalSize; start += chunkSize {
		end := chunkSize + start - 1
		if end > totalSize-1 {
			end = totalSize - 1
		}

		chunks = append(chunks, chunk{start, end, make(chan []byte, 1)})
	}

	return chunks
}

func ReturnJustString(data []byte, err error) string {
	return string(data)
}

func (DL *Youtube) getReq(URL, SIG, CT string) YTRequest {
	if URL != "" {
		if int_value, err := strconv.Atoi(CT); err == nil {
			return YTRequest{
				VideoID:       DL.ID,
				ContentLength: strconv.Itoa(int_value),
				URL:           URL,
				Config:        *DL,
			}
		}
	}
	if SIG != "" {
		if CL, Url, err := getSigUrlAndToken(SIG, DL.ID); err == nil {
			if CT == "" {
				CT = strconv.Itoa(CL)
			}
			return YTRequest{
				VideoID:       DL.ID,
				ContentLength: CT,
				URL:           Url,
				Config:        *DL,
				Sig:           true,
			}
		}
	}
	return YTRequest{}
}

func getSigUrlAndToken(SIG, VideoID string) (int, string, error) {
	pars, err := url.ParseQuery(SIG)
	if err != nil {
		return 0, "", err
	}

	u, err := url.Parse(pars.Get("url"))
	if err != nil {
		return 0, "", err
	}

	S, err := url.PathUnescape(pars.Get("s"))
	if err != nil {
		return 0, "", err
	}

	a, err := decrypt([]byte(S), VideoID)
	if err != nil {
		return 0, "", err
	}

	S = string(a)

	// decode S and get the token.
	q := u.Query()

	config, _ := getPlayerConfig(VideoID)
	q.Add(pars.Get("sp"), S)

	vals, err := decryptNParam(config, q)
	if err != nil {
		return 0, "", err
	}

	u.RawQuery = vals.Encode()

	URL := u.String()

	// perform the request to get the content length, as sometimes with the sig value requests it doesnt give one in the json itself.
	resp, err := http.Get(URL)
	if err != nil {
		return 0, "", err
	}

	ContentL, err := strconv.Atoi(resp.Header.Get("Content-Length"))
	if err != nil {
		return 0, "", err
	}

	return ContentL, URL, err
}

func Write(Vid []byte, filename string) {
	file, _ := os.Create(filename)
	file.Write(Vid)
	file.Close()
}

func getTitle(Runs []struct {
	Text string "json:\"text\""
}) (fill string) {
	for _, t := range Runs {
		fill += t.Text
	}
	return
}

func (YT YTRequest) GetStream(inp, out string, body []byte) *beep.Buffer {
	if ffmpeg, ok := checkFFM(); ok {
		Write(body, inp)

		dmo := exec.Command(ffmpeg, "-y", "-loglevel", "quiet", "-i", inp, "-vn", out)
		dmo.Run()

		f, _ := os.ReadFile(out)
		os.Remove(inp)
		os.Remove(out)

		songbody := io.NopCloser(bytes.NewBuffer(f))
		defer songbody.Close()
		streamer, format, err := mp3.Decode(songbody)
		if err != nil {
			return nil
		}

		buf := beep.NewBuffer(format)
		buf.Append(streamer)

		return buf
	}
	return nil
}

var PlayingRN bool

func (YT YTRequest) PlayWithStream(s *beep.Buffer) {
	speaker.Init(s.Format().SampleRate, s.Format().SampleRate.N(time.Second/10))
	done := make(chan bool)
	speaker.Play(beep.Seq(s.Streamer(0, s.Len()), beep.Callback(func() {
		done <- true
	})))
	<-done
	speaker.Clear()
	speaker.Close()
	PlayingRN = false
}

func (YT YTRequest) Play(inp, out string, body []byte) *beep.Buffer {
	if s := YT.GetStream(inp, out, body); s != nil {
		speaker.Init(s.Format().SampleRate, s.Format().SampleRate.N(time.Second/10))
		done := make(chan bool)
		speaker.Play(beep.Seq(s.Streamer(0, s.Len()), beep.Callback(func() {
			done <- true
		})))
		<-done
		speaker.Clear()
		speaker.Close()
		return s
	}
	return nil
}

func checkFFM() (string, bool) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		if runtime.GOOS == "windows" {
			if err := exec.Command("winget", "install", "ffmpeg").Run(); err != nil {
				return ffmpeg, false
			} else {
				ffmpeg, _ = exec.LookPath("ffmpeg")
			}
		}
		return ffmpeg, true
	}
	return ffmpeg, true
}
