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
	"strconv"
	"syscall"
	"time"

	"github.com/godump/cron"
	"github.com/godump/doa"
	"github.com/godump/gracefulexit"
)

type PirateConf struct {
	Capacity       int
	Category       int
	DataPath       string
	MaxUploadLimit string
	MaxWorker      int
	SeedTime       int
	SeedRatio      int
}

var (
	cConfPath = func() string {
		return filepath.Join(filepath.Dir(doa.Try(os.Executable())), "pirate.json")
	}()
	cConf = func() *PirateConf {
		conf := &PirateConf{}
		f := doa.Try(os.Open(cConfPath))
		defer f.Close()
		doa.Nil(json.NewDecoder(f).Decode(conf))
		return conf
	}()
)

type AriaClient struct {
	Add  time.Time
	Cmd  *exec.Cmd
	Name string
	Size int
}

type PirateItem struct {
	InfoHash string `json:"info_hash"`
	Name     string `json:"name"`
	Size     string `json:"size"`
}

type PirateDaze struct {
	Aria2c   []*AriaClient
	Browse   []*PirateItem
	Capacity int
	DataPath string
}

func (d *PirateDaze) Find(name string) (b bool) {
	for _, e := range d.Aria2c {
		b = e.Name == name
		if b {
			break
		}
	}
	return
}

func (d *PirateDaze) Size() (size int) {
	for _, e := range d.Aria2c {
		size += e.Size
	}
	return
}

func (d *PirateDaze) Data() {
	for {
		r, err := http.Get(fmt.Sprintf("https://apibay.org/q.php?q=category:%d", cConf.Category))
		if err != nil {
			log.Println("main:", err)
			time.Sleep(time.Second * 8)
			continue
		}
		data := doa.Try(ioutil.ReadAll(r.Body))
		doa.Nil(json.Unmarshal(data, &d.Browse))
		r.Body.Close()
		break
	}
}

func (d *PirateDaze) Scan() {
	arr := []*AriaClient{}
	for _, e := range d.Aria2c {
		if e.Cmd.ProcessState != nil {
			log.Println("main: exit", e.Name)
			os.RemoveAll(filepath.Join(d.DataPath, e.Name))
			os.Remove(filepath.Join(d.DataPath, e.Name+".aria2"))
			continue
		}
		arr = append(arr, e)
	}
	d.Aria2c = arr
}

func (d *PirateDaze) Join() {
	sum := d.Size()
	for _, e := range d.Browse {
		size := doa.Try(strconv.Atoi(e.Size))
		if len(d.Aria2c) >= cConf.MaxWorker {
			continue
		}
		if sum+size > d.Capacity {
			continue
		}
		if d.Find(e.Name) {
			continue
		}
		sum += size
		log.Println("main: join", e.Name)
		// Doc: https://aria2.github.io/manual/en/html/aria2c.html
		args := []string{
			fmt.Sprintf("--dir=%s", e.Name),
			fmt.Sprintf("--max-upload-limit=%s", cConf.MaxUploadLimit),
			fmt.Sprintf("--seed-ratio=%d", cConf.SeedRatio),
			fmt.Sprintf("--seed-time=%d", cConf.SeedTime),
			fmt.Sprintf("magnet:?xt=urn:btih:%s", e.InfoHash),
		}
		cmd := exec.Command("aria2c", args...)
		cmd.Dir = d.DataPath
		go cmd.Run()
		d.Aria2c = append(d.Aria2c, &AriaClient{
			Add:  time.Now(),
			Cmd:  cmd,
			Name: e.Name,
			Size: size,
		})
	}
}

func (d *PirateDaze) Exit() {
	for _, e := range d.Aria2c {
		e.Cmd.Process.Signal(syscall.SIGINT)
	}
	for _, e := range d.Aria2c {
		for e.Cmd.ProcessState == nil {
			time.Sleep(time.Second)
		}
	}
	d.Scan()
}

func NewDazePirate() *PirateDaze {
	return &PirateDaze{
		Aria2c: []*AriaClient{},
		Browse: []*PirateItem{},
	}
}

func main() {
	rand.Seed(time.Now().UnixNano())
	flag.Parse()
	_, err := exec.LookPath("aria2c")
	if err != nil {
		log.Println("main: aria2c not found, checkout https://aria2.github.io/ for how to install it.")
		return
	}
	p, err := func() (string, error) {
		if filepath.IsAbs(cConf.DataPath) {
			return cConf.DataPath, nil
		} else {
			return filepath.Abs(filepath.Join(filepath.Dir(doa.Try(os.Executable())), cConf.DataPath))
		}
	}()
	if err != nil {
		log.Println("main:", err)
		return
	}
	daze := NewDazePirate()
	daze.Capacity = 1024 * 1024 * 1024 * cConf.Capacity
	daze.DataPath = p
	doa.Nil(os.MkdirAll(daze.DataPath, 0755))
	if len(doa.Try(ioutil.ReadDir(daze.DataPath))) != 0 {
		log.Println("main:", daze.DataPath, "is not empty")
		return
	}
	daze.Data()
	daze.Join()
	chanPing := cron.Cron(time.Minute * time.Duration(30+rand.Int63n(30)))
	chanExit := gracefulexit.Chan()
	done := 0
	log.Println("main: loop")
	for {
		select {
		case <-chanPing:
			daze.Scan()
			daze.Data()
			daze.Join()
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
