package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
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
	fDataPath              = flag.String("d", "./pirate-data", "data path")
	cCapacity       uint64 = 1024 * 1024 * 1024 * 8
	cSeedTime              = 60 * 24
	cSeedRatio             = 8
	cMaxUploadLimit        = "128K"
)

type AriaClient struct {
	Add  time.Time
	Cmd  *exec.Cmd
	Name string
	Size uint64
}

type PirateItem struct {
	InfoHash string `json:"info_hash"`
	Name     string `json:"name"`
	Size     uint64 `json:"size"`
}

type PirateDaze struct {
	Aria2c   []*AriaClient
	Browse   []*PirateItem
	Capacity uint64
	DataPath string
}

func (d *PirateDaze) Search() {
	r := doa.Try(http.Get("https://apibay.org/precompiled/data_top100_recent.json"))
	defer r.Body.Close()
	data := doa.Try(ioutil.ReadAll(r.Body))
	doa.Nil(json.Unmarshal(data, &d.Browse))
}

func (d *PirateDaze) Remove() {
	arr := []*AriaClient{}
	for _, e := range d.Aria2c {
		if e.Cmd.ProcessState.Exited() {
			log.Println("main: exit", e.Name)
			doa.Nil(os.RemoveAll(filepath.Join(d.DataPath, e.Name)))
			continue
		}
		arr = append(arr, e)
	}
	d.Aria2c = arr
}

func (d *PirateDaze) Create() {
	size := uint64(0)
	for _, e := range d.Aria2c {
		size += e.Size
	}
	for _, e := range d.Browse {
		if size+e.Size > d.Capacity {
			continue
		}
		for _, f := range d.Aria2c {
			if e.Name == f.Name {
				continue
			}
		}
		size += e.Size
		log.Println("main: join", e.Name)
		// Doc: https://aria2.github.io/manual/en/html/aria2c.html
		args := []string{
			fmt.Sprintf("--max-upload-limit=%s", cMaxUploadLimit),
			fmt.Sprintf("--seed-ratio=%d", cSeedRatio),
			fmt.Sprintf("--seed-time=%d", cSeedTime),
			fmt.Sprintf("magnet:?xt=urn:btih:%s", e.InfoHash),
		}
		cmd := exec.Command("aria2c", args...)
		cmd.Dir = d.DataPath
		cmd.Start()
		d.Aria2c = append(d.Aria2c, &AriaClient{
			Add:  time.Now(),
			Cmd:  cmd,
			Name: e.Name,
			Size: e.Size,
		})
	}
}

func (d *PirateDaze) Exit() {
	for _, e := range d.Aria2c {
		log.Println("main: exit", e.Name)
		e.Cmd.Process.Signal(syscall.SIGINT)
		e.Cmd.Wait()
		doa.Nil(os.RemoveAll(filepath.Join(d.DataPath, e.Name)))
	}
}

func NewDazePirate() *PirateDaze {
	return &PirateDaze{
		Aria2c: []*AriaClient{},
		Browse: []*PirateItem{},
	}
}

func main() {
	_, err := exec.LookPath("aria2c")
	if err != nil {
		log.Println("main: aria2c not found, checkout https://aria2.github.io/ for how to install it.")
		return
	}
	p, err := func() (string, error) {
		if filepath.IsAbs(*fDataPath) {
			return *fDataPath, nil
		} else {
			return filepath.Abs(filepath.Join(filepath.Dir(doa.Try(os.Executable())), *fDataPath))
		}
	}()
	if err != nil {
		log.Println("main:", err)
	}
	daze := NewDazePirate()
	daze.Capacity = cCapacity
	daze.DataPath = p
	doa.Nil(os.MkdirAll(daze.DataPath, 0755))
	if len(doa.Try(ioutil.ReadDir(daze.DataPath))) != 0 {
		log.Println("main:", daze.DataPath, "is not empty")
		return
	}
	daze.Search()
	daze.Create()
	chanPing := cron.Cron(time.Hour)
	chanExit := gracefulexit.Chan()
	done := 0
	log.Println("main: loop")
	for {
		select {
		case <-chanPing:
			daze.Remove()
			daze.Search()
			daze.Create()
		case <-chanExit:
			daze.Exit()
			done = 1
		}
		if done != 0 {
			break
		}
	}
	log.Println("main: done")
}
