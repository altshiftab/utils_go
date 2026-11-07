package cloud_storage

import (
	"encoding/json/v2"
	"fmt"
	"hash/crc32"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_storage/cloud_storage_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_storage/types/object"
)

// fakeGcs stands in for Cloud Storage's resumable uploads, media reads and
// conditional deletes, keeping the state the real service keeps: what each
// session holds, which generation each name is at, and what a declared CRC32C
// or precondition does to an upload as it completes.
type fakeGcs struct {
	t      *testing.T
	server *httptest.Server

	mu       sync.Mutex
	objects  map[string]*fakeObject
	sessions map[string]*fakeSession
	nextId   int
	nextGen  int64

	initiations   int
	statusQueries int
	// chunkOffsets is the start offset of every PUT that carried bytes, in order.
	chunkOffsets []int64
	// failChunk says what the Nth PUT carrying bytes (1-based) meets instead
	// of an ordinary answer.
	failChunk map[int]*chunkFailure
	// onChunk is called with the count of PUTs carrying bytes, before each is
	// handled.
	onChunk func(n int)

	// mediaRequests records each media GET.
	mediaRequests []*mediaRequest
	// cutReads is how many media GETs have their connection dropped after
	// cutAfter bytes of the body.
	cutReads int
	cutAfter int
	// ignoreRange answers every media GET with the whole object and a 200.
	ignoreRange bool
	// onMedia is called with the count of media GETs, before each is handled.
	onMedia func(n int)
}

type fakeObject struct {
	data       []byte
	generation int64
}

type fakeSession struct {
	name     string
	size     int64
	crc32c   string
	ifAbsent bool
	data     []byte
	done     *object.Object
}

type chunkFailure struct {
	// status is answered instead; zero drops the connection with no answer.
	status int
	// keep stores the chunk before failing, as when an answer is lost after
	// the bytes were kept.
	keep bool
}

type mediaRequest struct {
	rangeHeader string
	generation  string
}

func newFakeGcs(t *testing.T) (*fakeGcs, *Client) {
	t.Helper()

	fake := &fakeGcs{
		t:         t,
		objects:   map[string]*fakeObject{},
		sessions:  map[string]*fakeSession{},
		failChunk: map[int]*chunkFailure{},
	}
	fake.server = httptest.NewServer(http.HandlerFunc(fake.handle))
	t.Cleanup(fake.server.Close)

	u, err := url.Parse(fake.server.URL)
	if err != nil {
		t.Fatalf("url parse: %v", err)
	}

	return fake, NewClient(cloud_storage_config.WithBaseUrl(u))
}

func (f *fakeGcs) put(name string, data []byte) *fakeObject {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.nextGen++
	stored := &fakeObject{data: append([]byte(nil), data...), generation: f.nextGen}
	f.objects[name] = stored

	return stored
}

func (f *fakeGcs) get(name string) *fakeObject {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.objects[name]
}

func crc32cString(data []byte) string {
	return EncodeCrc32c(crc32.Checksum(data, crc32cTable))
}

func (o *fakeObject) metadata(name string) *object.Object {
	return &object.Object{
		Name:       name,
		Generation: strconv.FormatInt(o.generation, 10),
		Size:       strconv.Itoa(len(o.data)),
		Crc32c:     crc32cString(o.data),
	}
}

func (f *fakeGcs) handle(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/upload/storage/v1/b/"):
		f.initiate(w, r)
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/upload/storage/v1/b/"):
		f.chunk(w, r)
	case r.Method == http.MethodGet && r.URL.Query().Get("alt") == "media":
		f.media(w, r)
	case r.Method == http.MethodDelete:
		f.delete(w, r)
	default:
		f.t.Errorf("unexpected request: %s %s", r.Method, r.URL)
		w.WriteHeader(http.StatusNotImplemented)
	}
}

func (f *fakeGcs) initiate(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.initiations++

	if r.URL.Query().Get("uploadType") != "resumable" {
		f.t.Errorf("unexpected upload type: %q", r.URL.Query().Get("uploadType"))
	}

	var metadata object.Object
	if err := json.UnmarshalRead(r.Body, &metadata); err != nil {
		f.t.Errorf("json unmarshal read (metadata): %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	size, err := strconv.ParseInt(r.Header.Get("X-Upload-Content-Length"), 10, 64)
	if err != nil {
		f.t.Errorf("upload content length: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	ifAbsent := r.URL.Query().Get("ifGenerationMatch") == "0"
	if ifAbsent && f.objects[metadata.Name] != nil {
		w.WriteHeader(http.StatusPreconditionFailed)
		return
	}

	f.nextId++
	id := fmt.Sprintf("s%d", f.nextId)
	f.sessions[id] = &fakeSession{name: metadata.Name, size: size, crc32c: metadata.Crc32c, ifAbsent: ifAbsent}

	w.Header().Set("Location", f.server.URL+r.URL.Path+"?uploadType=resumable&upload_id="+id)
	w.WriteHeader(http.StatusOK)
}

func (f *fakeGcs) chunk(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		f.t.Errorf("io read all (chunk): %v", err)
		return
	}

	f.mu.Lock()
	session := f.sessions[r.URL.Query().Get("upload_id")]
	if session == nil {
		f.mu.Unlock()
		w.WriteHeader(http.StatusNotFound)
		return
	}

	contentRange := r.Header.Get("Content-Range")
	span, totalString, found := strings.Cut(strings.TrimPrefix(contentRange, "bytes "), "/")
	total, err := strconv.ParseInt(totalString, 10, 64)
	if !found || err != nil || total != session.size {
		f.mu.Unlock()
		f.t.Errorf("bad content range: %q", contentRange)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if span == "*" {
		f.statusQueries++
		if len(body) != 0 {
			f.t.Errorf("status query with a body of %d bytes", len(body))
		}
		f.answer(w, r, session)
		return
	}

	startString, endString, _ := strings.Cut(span, "-")
	start, startErr := strconv.ParseInt(startString, 10, 64)
	end, endErr := strconv.ParseInt(endString, 10, 64)
	if startErr != nil || endErr != nil || end-start+1 != int64(len(body)) {
		f.mu.Unlock()
		f.t.Errorf("content range %q for a body of %d bytes", contentRange, len(body))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	f.chunkOffsets = append(f.chunkOffsets, start)
	n := len(f.chunkOffsets)
	onChunk := f.onChunk
	f.mu.Unlock()
	if onChunk != nil {
		onChunk(n)
	}
	f.mu.Lock()

	// The service takes a chunk only where the session's bytes end; a client
	// that sends from anywhere else has lost track of what was kept.
	if start != int64(len(session.data)) {
		f.mu.Unlock()
		f.t.Errorf("chunk starts at %d, but the session holds %d bytes", start, len(session.data))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if failure := f.failChunk[n]; failure != nil {
		if failure.keep {
			session.data = append(session.data, body...)
		}
		f.mu.Unlock()
		if failure.status == 0 {
			f.drop(w)
			return
		}
		w.WriteHeader(failure.status)
		return
	}

	session.data = append(session.data, body...)
	f.answer(w, r, session)
}

// answer reports where a session stands, completing it when it holds every
// byte. It is called with the lock held and releases it.
func (f *fakeGcs) answer(w http.ResponseWriter, r *http.Request, session *fakeSession) {
	if session.done != nil {
		done := session.done
		f.mu.Unlock()
		writeJson(f.t, w, http.StatusOK, done)
		return
	}

	if int64(len(session.data)) < session.size {
		if len(session.data) > 0 {
			w.Header().Set("Range", fmt.Sprintf("bytes=0-%d", len(session.data)-1))
		}
		f.mu.Unlock()
		w.WriteHeader(http.StatusPermanentRedirect)
		return
	}

	delete(f.sessions, r.URL.Query().Get("upload_id"))

	if session.crc32c != "" && session.crc32c != crc32cString(session.data) {
		f.mu.Unlock()
		writeJson(f.t, w, http.StatusBadRequest, map[string]string{"message": "Provided CRC32C does not match"})
		return
	}
	if session.ifAbsent && f.objects[session.name] != nil {
		f.mu.Unlock()
		w.WriteHeader(http.StatusPreconditionFailed)
		return
	}

	f.nextGen++
	stored := &fakeObject{data: session.data, generation: f.nextGen}
	f.objects[session.name] = stored
	session.done = stored.metadata(session.name)
	f.mu.Unlock()

	writeJson(f.t, w, http.StatusOK, session.done)
}

func (f *fakeGcs) media(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/storage/v1/b/bucket/o/")

	f.mu.Lock()
	f.mediaRequests = append(f.mediaRequests, &mediaRequest{
		rangeHeader: r.Header.Get("Range"),
		generation:  r.URL.Query().Get("generation"),
	})
	n := len(f.mediaRequests)
	onMedia := f.onMedia
	f.mu.Unlock()
	if onMedia != nil {
		onMedia(n)
	}

	f.mu.Lock()
	stored := f.objects[name]
	if stored == nil {
		f.mu.Unlock()
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if generation := r.URL.Query().Get("generation"); generation != "" && generation != strconv.FormatInt(stored.generation, 10) {
		f.mu.Unlock()
		w.WriteHeader(http.StatusNotFound)
		return
	}
	data := stored.data
	cut := f.cutReads > 0
	if cut {
		f.cutReads--
	}
	f.mu.Unlock()

	size := int64(len(data))
	w.Header().Set("X-Goog-Generation", strconv.FormatInt(stored.generation, 10))
	w.Header().Set("X-Goog-Hash", "crc32c="+crc32cString(data)+",md5=AAAAAAAAAAAAAAAAAAAAAA==")
	w.Header().Set("X-Goog-Stored-Content-Length", strconv.FormatInt(size, 10))

	start, end := int64(0), size
	status := http.StatusOK
	if rangeHeader := r.Header.Get("Range"); rangeHeader != "" && !f.ignoreRange {
		startString, endString, _ := strings.Cut(strings.TrimPrefix(rangeHeader, "bytes="), "-")
		var err error
		if start, err = strconv.ParseInt(startString, 10, 64); err != nil {
			f.t.Errorf("bad range: %q", rangeHeader)
		}
		if endString != "" {
			last, err := strconv.ParseInt(endString, 10, 64)
			if err != nil {
				f.t.Errorf("bad range: %q", rangeHeader)
			}
			end = min(last+1, size)
		}
		if start >= size {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", size))
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end-1, size))
		status = http.StatusPartialContent
	}

	w.Header().Set("Content-Length", strconv.FormatInt(end-start, 10))
	w.WriteHeader(status)

	if cut && int64(f.cutAfter) < end-start {
		_, _ = w.Write(data[start : start+int64(f.cutAfter)])
		// Out to the client first: the bytes before the break are the point.
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		f.drop(w)
		return
	}
	_, _ = w.Write(data[start:end])
}

func (f *fakeGcs) delete(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/storage/v1/b/bucket/o/")

	f.mu.Lock()
	defer f.mu.Unlock()

	stored := f.objects[name]
	if stored == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.URL.Query().Get("ifGenerationMatch") != strconv.FormatInt(stored.generation, 10) {
		w.WriteHeader(http.StatusPreconditionFailed)
		return
	}

	delete(f.objects, name)
	w.WriteHeader(http.StatusNoContent)
}

// drop ends the connection without finishing the response.
func (f *fakeGcs) drop(w http.ResponseWriter) {
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		f.t.Errorf("the response writer cannot be hijacked")
		return
	}
	conn, _, err := hijacker.Hijack()
	if err != nil {
		f.t.Errorf("hijack: %v", err)
		return
	}
	_ = conn.Close()
}

func writeJson(t *testing.T, w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.MarshalWrite(w, value); err != nil {
		t.Errorf("json marshal write: %v", err)
	}
}
