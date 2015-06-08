package shard

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path"
	"runtime/debug"
	"strings"
	"testing"
	"testing/quick"

	"github.com/pachyderm/pfs/lib/etcache"
	"github.com/pachyderm/pfs/lib/router"
	"github.com/pachyderm/pfs/lib/traffic"
)

func check(err error, t *testing.T) {
	if err != nil {
		debug.PrintStack()
		t.Fatal(err)
	}
}

func checkResp(res *http.Response, expected string, t *testing.T) {
	if res.StatusCode != 200 {
		debug.PrintStack()
		t.Fatalf("Got error status: %s", res.Status)
	}
	value, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	check(err, t)
	if string(value) != expected {
		t.Fatalf("Body:\n%s\ndidn't match:\n%s\n", string(value), expected)
	}
}

func writeFile(url, name, branch, data string, t *testing.T) {
	res, err := http.Post(url+path.Join("/file", name)+"?branch="+branch, "application/text", strings.NewReader(data))
	check(err, t)
	checkResp(res, fmt.Sprintf("Created %s, size: %d.\n", name, len(data)), t)
}

func checkFile(url, name, commit, data string, t *testing.T) {
	res, err := http.Get(url + path.Join("/file", name) + "?commit=" + commit)
	check(err, t)
	checkResp(res, data, t)
}

func checkNoFile(url, name, commit string, t *testing.T) {
	res, err := http.Get(url + path.Join("/file", name) + "?commit=" + commit)
	check(err, t)
	if res.StatusCode != 404 {
		debug.PrintStack()
		t.Fatalf("File: %s at commit: %s should have returned 404 but returned %s.", name, commit, res.Status)
	}
}

func commit(url, commit, branch string, t *testing.T) {
	_url := fmt.Sprintf("%s/commit?branch=%s&commit=%s", url, branch, commit)
	res, err := http.Post(_url, "", nil)
	check(err, t)
	checkResp(res, fmt.Sprintf("%s\n", commit), t)
}

func branch(url, commit, branch string, t *testing.T) {
	_url := fmt.Sprintf("%s/branch?branch=%s&commit=%s", url, branch, commit)
	res, err := http.Post(_url, "", nil)
	check(err, t)
	checkResp(res, fmt.Sprintf("Created branch. (%s) -> %s.\n", commit, branch), t)
}

func runOp(url string, o traffic.Op, t *testing.T) {
	switch {
	case o.Object == traffic.File && o.RW == traffic.W:
		writeFile(url, o.Path, o.Branch, o.Data, t)
	case o.Object == traffic.File && o.RW == traffic.R:
		checkFile(url, o.Path, o.Commit, o.Data, t)
	case o.Object == traffic.Commit:
		commit(url, o.Commit, o.Branch, t)
	case o.Object == traffic.Branch:
		branch(url, o.Commit, o.Branch, t)
	default:
		t.Fatal("Unrecognized op.")
	}
}

func runWorkload(url string, w traffic.Workload, t *testing.T) {
	for _, o := range w {
		runOp(url, o, t)
	}
}

func TestPing(t *testing.T) {
	shard := NewShard("TestPingData", "TestPingComp", "TestPingPipelines", 0, 1)
	check(shard.EnsureRepos(), t)
	s := httptest.NewServer(shard.ShardMux())
	defer s.Close()

	res, err := http.Get(s.URL + "/ping")
	check(err, t)
	checkResp(res, "pong\n", t)
	res.Body.Close()
}

func TestBasic(t *testing.T) {
	c := 0
	f := func(w traffic.Workload) bool {
		shard := NewShard(fmt.Sprintf("TestBasic%d", c), fmt.Sprintf("TestBasicComp%d", c), fmt.Sprintf("TestBasicPipelines%d", c), 0, 1)
		c++
		check(shard.EnsureRepos(), t)
		s := httptest.NewServer(shard.ShardMux())
		defer s.Close()

		runWorkload(s.URL, w, t)
		facts := w.Facts()
		runWorkload(s.URL, facts, t)
		return true
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 5}); err != nil {
		t.Error(err)
	}
}

func TestPull(t *testing.T) {
	log.SetFlags(log.Lshortfile)
	c := 0
	f := func(w traffic.Workload) bool {
		_src := NewShard(fmt.Sprintf("TestPullSrc%d", c), fmt.Sprintf("TestPullSrcComp%d", c), fmt.Sprintf("TestPullSrcPipelines%d", c), 0, 1)
		_dst := NewShard(fmt.Sprintf("TestPullDst%d", c), fmt.Sprintf("TestPullDstComp%d", c), fmt.Sprintf("TestPullDstPipelines%d", c), 0, 1)
		c++
		check(_src.EnsureRepos(), t)
		check(_dst.EnsureRepos(), t)
		src := httptest.NewServer(_src.ShardMux())
		dst := httptest.NewServer(_dst.ShardMux())
		defer src.Close()
		defer dst.Close()

		runWorkload(src.URL, w, t)

		// Replicate the data
		srcReplica := NewShardReplica(src.URL)
		dstReplica := NewShardReplica(dst.URL)
		err := srcReplica.Pull("", dstReplica)
		check(err, t)
		facts := w.Facts()
		runWorkload(dst.URL, facts, t)
		return true
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 5}); err != nil {
		t.Error(err)
	}
}

// TestSync is similar to TestPull but it does it syncs after every commit.
func TestSyncTo(t *testing.T) {
	log.SetFlags(log.Lshortfile)
	c := 0
	f := func(w traffic.Workload) bool {
		_src := NewShard(fmt.Sprintf("TestSyncToSrc%d", c), fmt.Sprintf("TestSyncToSrcComp%d", c), fmt.Sprintf("TestSyncToSrcPipelines%d", c), 0, 1)
		_dst := NewShard(fmt.Sprintf("TestSyncToDst%d", c), fmt.Sprintf("TestSyncToDstComp%d", c), fmt.Sprintf("TestSyncToDstPipelines%d", c), 0, 1)
		check(_src.EnsureRepos(), t)
		check(_dst.EnsureRepos(), t)
		src := httptest.NewServer(_src.ShardMux())
		dst := httptest.NewServer(_dst.ShardMux())
		defer src.Close()
		defer dst.Close()

		for _, o := range w {
			runOp(src.URL, o, t)
			if o.Object == traffic.Commit {
				// Replicate the data
				err := SyncTo(fmt.Sprintf("TestSyncToSrc%d", c), []string{dst.URL})
				check(err, t)
			}
		}

		facts := w.Facts()
		runWorkload(dst.URL, facts, t)

		c++
		return true
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 5}); err != nil {
		t.Error(err)
	}
}

// TestSyncFrom
func TestSyncFrom(t *testing.T) {
	log.SetFlags(log.Lshortfile)
	c := 0
	f := func(w traffic.Workload) bool {
		_src := NewShard(fmt.Sprintf("TestSyncFromSrc%d", c), fmt.Sprintf("TestSyncFromSrcComp%d", c), fmt.Sprintf("TestSyncFromSrcPipelines%d", c), 0, 1)
		_dst := NewShard(fmt.Sprintf("TestSyncFromDst%d", c), fmt.Sprintf("TestSyncFromDstComp%d", c), fmt.Sprintf("TestSyncFromDstPipelines%d", c), 0, 1)
		check(_src.EnsureRepos(), t)
		check(_dst.EnsureRepos(), t)
		src := httptest.NewServer(_src.ShardMux())
		dst := httptest.NewServer(_dst.ShardMux())
		defer src.Close()
		defer dst.Close()

		for _, o := range w {
			runOp(src.URL, o, t)
			if o.Object == traffic.Commit {
				// Replicate the data
				err := SyncFrom(fmt.Sprintf("TestSyncFromDst%d", c), []string{src.URL})
				check(err, t)
			}
		}

		facts := w.Facts()
		runWorkload(dst.URL, facts, t)

		c++
		return true
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 5}); err != nil {
		t.Error(err)
	}
}

// TestPipeline creates a basic pipeline on a shard.
func TestPipeline(t *testing.T) {
	log.SetFlags(log.Lshortfile)
	shard := NewShard("TestPipelineData", "TestPipelineComp", "TestPipelinePipelines", 0, 1)
	check(shard.EnsureRepos(), t)
	s := httptest.NewServer(shard.ShardMux())
	defer s.Close()

	res, err := http.Post(s.URL+"/pipeline/touch_foo", "application/text", strings.NewReader(`
image ubuntu

run touch /out/foo
`))
	check(err, t)
	res.Body.Close()

	res, err = http.Post(s.URL+"/commit?commit=commit1", "", nil)
	check(err, t)
	checkFile(s.URL+"/pipeline/touch_foo", "foo", "commit1", "", t)
}

// TestShardFilter creates a basic pipeline on a shard and then requests files
// from it using shard filtering.

func TestShardFilter(t *testing.T) {
	log.SetFlags(log.Lshortfile)
	shard := NewShard("TestShardFilterData", "TestShardFilterComp", "TestShardFilterPipelines", 0, 1)
	check(shard.EnsureRepos(), t)
	s := httptest.NewServer(shard.ShardMux())
	defer s.Close()

	res, err := http.Post(s.URL+"/pipeline/files", "application/text", strings.NewReader(`
image ubuntu

run touch /out/foo
run touch /out/bar
run touch /out/buzz
run touch /out/bizz
`))
	check(err, t)
	res.Body.Close()

	res, err = http.Post(s.URL+"/commit?commit=commit1", "", nil)
	check(err, t)

	// Map to store files we receive
	files := make(map[string]struct{})
	res, err = http.Get(s.URL + path.Join("/pipeline", "files", "file", "*") + "?commit=commit1&shard=0-2")
	check(err, t)
	if res.StatusCode != 200 {
		t.Fatal(res.Status)
	}
	reader := multipart.NewReader(res.Body, res.Header.Get("Boundary"))

	for p, err := reader.NextPart(); err != io.EOF; p, err = reader.NextPart() {
		match, err := router.Match(p.FileName(), "0-2")
		check(err, t)
		if !match {
			t.Fatalf("Filename: %s should match.", p.FileName())
		}
		if _, ok := files[p.FileName()]; ok == true {
			t.Fatalf("File: %s received twice.")
		}
		files[p.FileName()] = struct{}{}
	}

	res, err = http.Get(s.URL + path.Join("/pipeline", "files", "file", "*") + "?commit=commit1&shard=1-2")
	check(err, t)
	if res.StatusCode != 200 {
		t.Fatal(res.Status)
	}
	reader = multipart.NewReader(res.Body, res.Header.Get("Boundary"))

	for p, err := reader.NextPart(); err != io.EOF; p, err = reader.NextPart() {
		match, err := router.Match(p.FileName(), "1-2")
		check(err, t)
		if !match {
			t.Fatalf("Filename: %s should match.", p.FileName())
		}
		if _, ok := files[p.FileName()]; ok == true {
			t.Fatalf("File: %s received twice.")
		}
		files[p.FileName()] = struct{}{}
	}
}

func TestShuffle(t *testing.T) {
	log.SetFlags(log.Lshortfile)

	// Setup 2 shards
	shard1 := NewShard("TestShuffleData-0-2", "TestShuffleComp-0-2", "TestShufflePipelines-0-2", 0, 2)
	check(shard1.EnsureRepos(), t)
	s1 := httptest.NewServer(shard1.ShardMux())
	defer s1.Close()
	shard2 := NewShard("TestShuffleData-1-2", "TestShuffleComp-1-2", "TestShufflePipelines-1-2", 1, 2)
	check(shard2.EnsureRepos(), t)
	s2 := httptest.NewServer(shard2.ShardMux())
	defer s2.Close()

	files := []string{"foo", "bar", "fizz", "buzz"}

	for _, file := range files {
		writeFile(s1.URL, path.Join("data", file), "master", file, t)
		writeFile(s2.URL, path.Join("data", file), "master", file, t)
	}

	// Spoof the shards in etcache
	etcache.SpoofMany("/pfs/master", []string{s1.URL, s2.URL}, false)

	pipeline := `
image ubuntu

input data

run cp -r /in/data /out

shuffle data
`
	res, err := http.Post(s1.URL+"/pipeline/shuffle", "application/text", strings.NewReader(pipeline))
	check(err, t)
	res.Body.Close()
	res, err = http.Post(s2.URL+"/pipeline/shuffle", "application/text", strings.NewReader(pipeline))
	check(err, t)
	res.Body.Close()

	res, err = http.Post(s1.URL+"/commit?commit=commit1", "", nil)
	check(err, t)
	res, err = http.Post(s2.URL+"/commit?commit=commit1", "", nil)
	check(err, t)

	for _, file := range files {
		match, err := router.Match(path.Join("data", file), "0-2")
		check(err, t)
		if match {
			log.Print("shard: s1 file: ", file)
			checkFile(s1.URL+"/pipeline/shuffle", path.Join("data", file), "commit1", file+file, t)
		} else {
			log.Print("shard: s2 file: ", file)
			checkFile(s2.URL+"/pipeline/shuffle", path.Join("data", file), "commit1", file+file, t)
		}
	}
}