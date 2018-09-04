// Crudely download and generate a list of builds from a deployment log.
//
// SPECIAL INSTRUCTIONS: Build version-0d11713edd8c4452 screwed up the
// location of ReflectionMetadata, and, as a result, is written incorrectly
// under usual circumstances. This can be fixed by moving
// ReflectionMetadata.xml to the following location, relative to the location
// of RobloxPlayerBeta.exe:
//
//     ../../../Client/ClientBase/
//
// TODO: This could be resolved by, 1) moving the cache a number of
// directories deeper, 2) detecting the problem build, 3) moving its
// ReflectionMetadata to the directory indicated above, which now wont be
// outside the main cache directory.
package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/robloxapi/rbxdhist"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Map Job to a corresponding Status.
func mapStatus(stream rbxdhist.Stream) map[*rbxdhist.Job]bool {
	m := map[*rbxdhist.Job]bool{}
	for i := 0; i < len(stream); i++ {
		job, ok := stream[i].(*rbxdhist.Job)
		if !ok || i+1 >= len(stream) {
			continue
		}
		if status, ok := stream[i+1].(*rbxdhist.Status); ok && string(*status) == "Done" {
			m[job] = true
			i++
		}
	}
	return m
}

func FilterStream(stream rbxdhist.Stream) (jobs []*rbxdhist.Job) {
	// Map jobs to statuses.
	status := mapStatus(stream)

	// Filter all non-jobs.
	for i := range stream {
		if job, ok := stream[i].(*rbxdhist.Job); ok && status[job] {
			jobs = append(jobs, job)
		}
	}

	// Convert revisions into rebuilds. A rebuild has the same hash and
	// version, but a different date.
	for i := 0; i < len(jobs); i++ {
		job := jobs[i]
		if job.Action == "Revert" {
			// Search for revert target.
			for j := i - 1; j > 0; j-- {
				if jobs[j].Build == job.Build && jobs[j].Hash == job.Hash {
					job.Action = "New"
					job.Version = jobs[j].Version
					break
				}
			}
		}
	}

	// Normalize build types.
	for i := range jobs {
		switch jobs[i].Build {
		case "Client", "WindowsPlayer":
			jobs[i].Build = "Player"
		case "Studio", "MFCStudio":
			jobs[i].Build = "Studio"
		}
	}

	return jobs
}

type Build struct {
	Hash          string
	SecondaryHash string `json:",omitempty"`
	Date          time.Time
	Version       rbxdhist.Version
}

func (b *Build) String() string {
	s := b.Hash + " " + b.Date.String() + " " + b.Version.String()
	if b.SecondaryHash != "" {
		s += " " + b.SecondaryHash
	}
	return s
}

type Environment struct {
	Dir string
}

func Execute(name string, args ...string) (err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).Run()
	// return exec.Command(name, args...).Run()
}

const MinFileSize int64 = 1 << 10

func Extractor(env *Environment, build *Build, archiveDir string) (err error) {
	// Ensure archive directories are present.
	if err := os.MkdirAll(filepath.Join(archiveDir, "data", "api-dump", "txt"), 0666); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(archiveDir, "data", "reflection-metadata", "xml"), 0666); err != nil {
		return err
	}

	apiOut := filepath.Join(archiveDir, "data", "api-dump", "txt", build.Hash+".txt")
	rmdOut := filepath.Join(archiveDir, "data", "reflection-metadata", "xml", build.Hash+".xml")
	if _, err := os.Stat(apiOut); !os.IsNotExist(err) {
		if _, err := os.Stat(rmdOut); !os.IsNotExist(err) {
			return nil
		}
	}

	// ReflectionMetadata
	rmdPath := filepath.Join(env.Dir, "ReflectionMetadata.xml")
	if fi, err := os.Stat(rmdPath); os.IsNotExist(err) {
		return err
	} else if fi.Size() < MinFileSize {
		return errors.New("ReflectionMetadata file too small.")
	}

	// API Dump
	apiName := "api.txt"
	apiPath := filepath.Join(env.Dir, apiName)
	if fi, err := os.Stat(apiPath); os.IsNotExist(err) || fi.Size() < MinFileSize {
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		if err := os.Chdir(env.Dir); err != nil {
			return err
		}
		exes := [][2]string{
			{"RobloxPlayerBeta.exe", "--API"},
			{"RobloxPlayer.exe", "-API"},
			{"RobloxApp.exe", "-API"},
		}
		for _, exe := range exes {
			if _, err := os.Stat(exe[0]); os.IsNotExist(err) {
				continue
			}
			if runtime.GOOS == "windows" {
				if err := Execute(exe[0], exe[1], apiName); err == nil {
					break
				}
			} else {
				if err := Execute("wine", exe[0], exe[1], apiName); err == nil {
					break
				}
			}
		}
		fi, err = os.Stat(apiName)
		os.Chdir(wd)
		if os.IsNotExist(err) {
			return err
		} else if fi.Size() < MinFileSize {
			return errors.New("API dump file size is too small")
		}
	}

	// Extract API Dump
	{
		in, err := os.Open(apiPath)
		if err != nil {
			return err
		}
		out, err := os.Create(apiOut)
		if err != nil {
			in.Close()
			return err
		}
		_, err = io.Copy(out, in)
		in.Close()
		out.Close()
		if err != nil {
			return err
		}
	}

	// Extract ReflectionMetadata
	{
		in, err := os.Open(rmdPath)
		if err != nil {
			return err
		}
		out, err := os.Create(rmdOut)
		if err != nil {
			in.Close()
			return err
		}
		_, err = io.Copy(out, in)
		in.Close()
		out.Close()
		if err != nil {
			return err
		}
	}

	return nil
}

// Unzip zip file at u to dstPath directory. The filters argument is a number
// of file names. If any filters are given, then only those files are
// unzipped, and Unzip errors if at least one of these files was not found.
func Unzip(dstPath string, u *url.URL, filters ...string) (err error) {
	resp, err := http.Get(u.String())
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		return errors.New("bad status: " + resp.Status)
	}
	tmp, err := ioutil.TempFile("", path.Base(u.Path))
	if err != nil {
		resp.Body.Close()
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	_, err = io.Copy(tmp, resp.Body)
	resp.Body.Close()
	if err != nil {
		return err
	}
	if _, err = tmp.Seek(0, 0); err != nil {
		return err
	}
	zr, err := zip.NewReader(tmp, resp.ContentLength)
	if err != nil {
		return err
	}
	filter := make(map[string]bool, len(filters))
	for _, f := range filters {
		filter[f] = false
	}
	for _, file := range zr.File {
		var ok bool
		if len(filter) > 0 {
			if _, ok = filter[file.Name]; !ok {
				continue
			}
		}
		if err := os.MkdirAll(filepath.Join(dstPath, filepath.Dir(file.Name)), 0666); err != nil {
			return err
		}
		src, err := file.Open()
		if err != nil {
			return err
		}
		dst, err := os.Create(filepath.Join(dstPath, file.Name))
		if err != nil {
			src.Close()
			return err
		}
		_, err = io.Copy(dst, src)
		src.Close()
		dst.Close()
		if err != nil {
			return err
		}
		if ok {
			filter[file.Name] = true
		}
	}
	for file, ok := range filter {
		if !ok {
			return errors.New("failed to unzip file " + file)
		}
	}
	return nil
}

func GetManifest(dir string, key string) (value string, ok bool) {
	man, err := ioutil.ReadFile(filepath.Join(dir, "manifest.txt"))
	if err != nil {
		return "", false
	}
	for _, entry := range strings.Split(string(man), "\n") {
		i := strings.Index(entry, ":")
		if i < 0 {
			i = len(entry)
		}
		if entry[:i] == key {
			return entry[i:], true
		}
	}
	return "", false
}

func SetManifest(dir string, key, value string) {
	manPath := filepath.Join(dir, "manifest.txt")
	var entries []string
	if man, err := ioutil.ReadFile(manPath); err == nil {
		entries = strings.Split(string(man), "\n")
	}
	e := entries[:0]
	for _, entry := range entries {
		i := strings.Index(entry, ":")
		if i < 0 {
			i = len(entry)
		}
		if entry[:i] != key {
			e = append(e, entry)
		}
	}
	entries = append(e, key+":"+value)
	ioutil.WriteFile(manPath, []byte(strings.Join(entries, "\n")), 0666)
}

type Config struct {
	Name string
	Func func(env *Environment, build *Build, host string, i int, jobs []*rbxdhist.Job) (err error)
}

var Configs = []Config{
	{"RobloxApp.zip", func(env *Environment, build *Build, host string, i int, jobs []*rbxdhist.Job) (err error) {
		if _, ok := GetManifest(env.Dir, "RobloxApp.zip"); ok {
			return nil
		}
		if err := Unzip(
			env.Dir,
			&url.URL{Scheme: "https", Host: host, Path: build.Hash + "-RobloxApp.zip"},
		); err != nil {
			return err
		}

		// AppSettings
		appSettings := []byte("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\r\n<Settings>\r\n\t<ContentFolder>content</ContentFolder>\r\n\t<BaseUrl>http://www.roblox.com</BaseUrl>\r\n</Settings>")
		if err := ioutil.WriteFile(filepath.Join(env.Dir, "AppSettings.xml"), appSettings, 0666); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Join(env.Dir, "content"), 0666); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Join(env.Dir, "PlatformContent", "pc"), 0666); err != nil {
			return err
		}

		SetManifest(env.Dir, "RobloxApp.zip", "")
		return nil
	}},
	{"Libraries.zip", func(env *Environment, build *Build, host string, i int, jobs []*rbxdhist.Job) (err error) {
		if _, ok := GetManifest(env.Dir, "Libraries.zip"); ok {
			return nil
		}
		if err := Unzip(
			env.Dir,
			&url.URL{Scheme: "https", Host: host, Path: build.Hash + "-Libraries.zip"},
		); err != nil {
			return err
		}
		SetManifest(env.Dir, "Libraries.zip", "")
		return nil
	}},
	{"redist.zip", func(env *Environment, build *Build, host string, i int, jobs []*rbxdhist.Job) (err error) {
		if _, ok := GetManifest(env.Dir, "redist.zip"); ok {
			return nil
		}
		if err := Unzip(
			env.Dir,
			&url.URL{Scheme: "https", Host: host, Path: build.Hash + "-redist.zip"},
		); err != nil {
			return err
		}
		SetManifest(env.Dir, "redist.zip", "")
		return nil
	}},
	{"ReflectionMetadata", func(env *Environment, build *Build, host string, i int, jobs []*rbxdhist.Job) (err error) {
		if v, ok := GetManifest(env.Dir, "ReflectionMetadata"); ok {
			build.SecondaryHash = v
			return nil
		}

		// Skip if ReflectionMetadata happens to exist.
		if _, err := os.Stat(filepath.Join(env.Dir, "ReflectionMetadata.xml")); !os.IsNotExist(err) {
			SetManifest(env.Dir, "ReflectionMetadata", "")
			return nil
		}

		// Get nearest studio build.
		var prev, next *rbxdhist.Job
		for j := i - 1; j >= 0; j-- {
			if jobs[j].Build == "Studio" {
				prev = jobs[j]
				break
			}
		}
		for j := i + 1; j < len(jobs); j++ {
			if jobs[j].Build == "Studio" {
				next = jobs[j]
				break
			}
		}
		switch {
		case prev != nil && next != nil:
			if next.Time.Sub(build.Date) < build.Date.Sub(prev.Time) {
				build.SecondaryHash = next.Hash
			} else {
				build.SecondaryHash = prev.Hash
			}
		case prev != nil:
			build.SecondaryHash = prev.Hash
		case next != nil:
			build.SecondaryHash = next.Hash
		default:
			return errors.New("no studio builds")
		}

		// Get chunk which contains ReflectionMetadata.
		if err := Unzip(
			env.Dir,
			&url.URL{Scheme: "https", Host: host, Path: build.SecondaryHash + "-RobloxStudio.zip"},
			"ReflectionMetadata.xml",
		); err != nil {
			return err
		}
		SetManifest(env.Dir, "ReflectionMetadata", build.SecondaryHash)
		return nil
	}},
}

func UserCacheDir() (dir string) {
	switch runtime.GOOS {
	case "windows":
		dir = os.Getenv("LocalAppData")
	default: // Unix
		dir = os.Getenv("XDG_CACHE_HOME")
		if dir == "" {
			dir = os.Getenv("HOME")
			dir += "/.cache"
		}
	}
	if dir == "" {
		dir = os.TempDir()
	}
	return dir
}

type Settings struct {
	CacheDir   string
	ArchiveDir string
	Host       string
	Builds     string
}

func main() {
	settings := Settings{}
	{
		settingsFile, err := os.Open("settings.json")
		if err != nil {
			fmt.Println(err)
			return
		}
		err = json.NewDecoder(settingsFile).Decode(&settings)
		settingsFile.Close()
		if err != nil {
			fmt.Println()
			return
		}
	}

	buildsFile, err := os.Create(filepath.Join(settings.ArchiveDir, "builds.json"))
	if err != nil {
		fmt.Println("failed to create builds file:", err)
		return
	}
	defer buildsFile.Close()

	latestFile, err := os.Create(filepath.Join(settings.ArchiveDir, "latest.json"))
	if err != nil {
		fmt.Println("failed to create latest file:", err)
		return
	}
	defer latestFile.Close()

	resp, err := http.Get((&url.URL{Scheme: "https", Host: settings.Host, Path: settings.Builds}).String())
	if err != nil {
		fmt.Println("failed to get deploy log:", err)
		return
	}
	b, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		fmt.Println("failed to read deploy log:", err)
		return
	}
	stream := rbxdhist.Lex(b)
	jobs := FilterStream(stream)
	var goodBuilds []*Build
	var badBuilds []*Build

	pst, _ := time.LoadLocation("America/Los_Angeles")
	dumpEpoch := time.Date(2011, 10, 25, 0, 0, 0, 0, pst)
	jsonEpoch := time.Date(2018, 8, 7, 0, 0, 0, 0, pst)

	for i, job := range jobs {
		if job.Build != "Player" || job.Time.After(jsonEpoch) || job.Time.Before(dumpEpoch) {
			continue
		}
		env := &Environment{
			Dir: filepath.Join(settings.CacheDir, job.Hash),
		}
		build := &Build{
			Hash:    job.Hash,
			Date:    job.Time,
			Version: job.Version,
		}
		fmt.Println("try:", build)
		var success bool
		for _, cfg := range Configs {
			if err := cfg.Func(env, build, settings.Host, i, jobs); err != nil {
				fmt.Println("\tconfig ", cfg.Name, err)
				break
			}
			err := Extractor(env, build, settings.ArchiveDir)
			if err == nil {
				success = true
				break
			}
			fmt.Println("\textract:", err)
		}
		if !success {
			fmt.Println("\tfailed:", build)
			badBuilds = append(badBuilds, build)
			continue
		}
		fmt.Println("\tsucceeded:", build)
		goodBuilds = append(goodBuilds, build)
	}

	je := json.NewEncoder(buildsFile)
	je.SetIndent("", "\t")
	je.SetEscapeHTML(false)
	if err := je.Encode(&goodBuilds); err != nil {
		fmt.Println("failed to encode builds:", err)
		return
	}
	je = json.NewEncoder(latestFile)
	je.SetIndent("", "")
	je.SetEscapeHTML(false)
	if err := je.Encode(&goodBuilds[len(goodBuilds)-1]); err != nil {
		fmt.Println("failed to encode latest:", err)
		return
	}
}
