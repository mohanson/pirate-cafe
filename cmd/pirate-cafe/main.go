package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/godump/cron"
	"github.com/godump/doa"
	"github.com/godump/gracefulexit"
)

var (
	fDataPath = flag.String("datapath", "./pirate", "")
	fCapacity = flag.Uint64("capacity", 1024*1024*1024*8, "")
)

type PirateItem struct {
	InfoHash string `json:"info_hash"`
	Name     string `json:"name"`
	Size     uint64 `json:"size"`
}

type PirateDaze struct {
	Aria2c   *exec.Cmd
	Browse   []PirateItem
	Capacity uint64
	DataPath string
}

func (d *PirateDaze) Delete() {
	for _, e := range doa.Try(ioutil.ReadDir(d.DataPath)) {
		p := filepath.Join(d.DataPath, e.Name())
		doa.Nil(os.RemoveAll(p))
	}
}

func (d *PirateDaze) Search() {
	r := doa.Try(http.Get("https://apibay.org/precompiled/data_top100_recent.json"))
	defer r.Body.Close()
	data := doa.Try(ioutil.ReadAll(r.Body))
	doa.Nil(json.Unmarshal(data, &d.Browse))
	rand.Shuffle(len(d.Browse), func(i, j int) {
		d.Browse[i], d.Browse[j] = d.Browse[j], d.Browse[i]
	})
}

func (d *PirateDaze) Update() {
	size := uint64(0)
	urls := []string{}
	for _, e := range d.Browse {
		s := e.Size
		if size+s > d.Capacity {
			continue
		}
		size += s
		hash := e.InfoHash
		name := e.Name
		log.Println("main:", hash, name)
		urls = append(urls, fmt.Sprintf("magnet:?xt=urn:btih:%s", hash))
	}
	// Doc: https://aria2.github.io/manual/en/html/aria2c.html
	args := []string{
		fmt.Sprintf("--max-concurrent-downloads=%d", len(urls)),
		"--max-overall-upload-limit=1M",
		"--max-upload-limit=128K",
		"--seed-ratio=0",
	}
	args = append(args, urls...)
	cmd := exec.Command("aria2c", args...)
	cmd.Dir = d.DataPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Start()
	d.Aria2c = cmd
}

func NewDazePirate() *PirateDaze {
	return &PirateDaze{
		Aria2c: nil,
		Browse: []PirateItem{},
	}
}

func main() {
	flag.Parse()
	_, err := exec.LookPath("aria2c")
	if err != nil {
		log.Println("main: aria2c not found, checkout https://aria2.github.io/ for how to install it.")
		return
	}
	daze := NewDazePirate()
	daze.Capacity = *fCapacity
	daze.DataPath = doa.Try(filepath.Abs(*fDataPath))
	doa.Nil(os.MkdirAll(daze.DataPath, 0755))
	if len(doa.Try(ioutil.ReadDir(daze.DataPath))) != 0 {
		log.Println("main:", daze.DataPath, "is not empty")
		return
	}
	daze.Search()
	daze.Update()
	chanPing := cron.Cron(time.Hour * 4)
	chanExit := gracefulexit.Chan()
	done := 0
	log.Println("main: loop")
	for {
		select {
		case <-chanPing:
			daze.Aria2c.Process.Signal(syscall.SIGINT)
			daze.Aria2c.Wait()
			daze.Delete()
			daze.Search()
			daze.Update()
		case <-chanExit:
			daze.Aria2c.Process.Signal(syscall.SIGINT)
			daze.Aria2c.Wait()
			daze.Delete()
			done = 1
		}
		if done != 0 {
			break
		}
	}
	log.Println("main: done")
}
