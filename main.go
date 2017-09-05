package main

import (
	"net/http"

	"encoding/json"
	"os"
	"regexp"

	"path"

	"crypto/md5"
	"image"
	"image/jpeg"
	"image/png"

	"encoding/hex"

	"log"

	"path/filepath"
	"strings"

	"errors"

	"fmt"
	"os/user"

	"flag"

	"io/ioutil"

	"github.com/nfnt/resize"
	"github.com/rs/cors"
	"goji.io"
	"goji.io/pat"
)

var version = "master"

func gophoPath(p string) string {
	usr, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}
	return path.Join(usr.HomeDir, ".gopho", p)
}

var roots = map[string]string{}

var rootIsSingleFolder = false

var dataPath = "/data"

var dataDir = flag.String("data-dir", gophoPath("data"), "path of data dir (must be writable)")
var uiDir = flag.String("ui-dir", gophoPath("ui"), "path of ui dir")

type entry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	IsDir   bool   `json:"isDir"`
	IsImage bool   `json:"isImage"`
}

var imageRe, pngRe, jpegRe *regexp.Regexp

func init() {
	var err error
	imageRe, err = regexp.Compile(`\.(jpg|jpeg|png)$`)
	if err != nil {
		panic(err)
	}
	pngRe, err = regexp.Compile(`\.png$`)
	if err != nil {
		panic(err)
	}
	jpegRe, err = regexp.Compile(`\.jpg|jpeg$`)
	if err != nil {
		panic(err)
	}
	if err := os.MkdirAll(*dataDir, 0755); err != nil {
		panic(err)
	}
}

func md5String(text string) string {
	hash := md5.Sum([]byte(text))
	return hex.EncodeToString(hash[:])
}

func addEntry(entries *[]entry, p string) error {
	file, err := os.Open(p)
	if err != nil {
		return err
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return err
	}
	if stat.IsDir() {
		*entries = append(*entries, entry{
			Name:  stat.Name(),
			Path:  getRelPath(p),
			Size:  stat.Size(),
			IsDir: true,
		})
		return nil
	}
	if imageRe.MatchString(p) {
		*entries = append(*entries, entry{
			Name:    stat.Name(),
			Path:    getRelPath(p),
			Size:    stat.Size(),
			IsImage: true,
		})
	}
	return nil
}

func getAbsPath(p string) (string, error) {
	if !filepath.IsAbs(p) {
		return "", errors.New("Invalid path")
	}
	if strings.HasPrefix(p, dataPath) {
		return strings.Replace(p, dataPath, *dataDir, 1), nil
	}
	for rel, abs := range roots {
		if strings.HasPrefix(p, rel) {
			return strings.Replace(p, rel, abs, 1), nil
		}
	}
	return "", errors.New("Invalid path")
}

func getRelPath(p string) string {
	if strings.HasPrefix(p, *dataDir) {
		return strings.Replace(p, *dataDir, dataPath, 1)
	}
	for rel, abs := range roots {
		if strings.HasPrefix(p, abs) {
			return strings.Replace(p, abs, rel, 1)
		}
	}
	log.Panicf("Invalid path %s", p)
	return ""
}

func makeThumb(p string) (string, error) {
	file, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := md5String(p)
	thumbPath := path.Join(*dataDir, hash+".jpg")
	metaPath := path.Join(*dataDir, hash+".json")
	if _, err := os.Stat(thumbPath); os.IsNotExist(err) {
		var img image.Image
		if pngRe.MatchString(p) {
			img, err = png.Decode(file)
		} else if jpegRe.MatchString(p) {
			img, err = jpeg.Decode(file)
		} else {
			return "", fmt.Errorf("%s: file must be JPEG or PNG", p)
		}
		if err != nil {
			return "", fmt.Errorf("%s: %s", p, err)
		}
		thumb := resize.Thumbnail(1200, 1200, img, resize.Bicubic)
		thumbOut, err := os.Create(thumbPath)
		if err != nil {
			return "", fmt.Errorf("%s: %s", thumbPath, err)
		}
		defer thumbOut.Close()
		jpeg.Encode(thumbOut, thumb, nil)

		metaOut, err := os.Create(metaPath)
		if err != nil {
			return "", fmt.Errorf("%s: %s", metaPath, err)
		}
		defer metaOut.Close()
		enc := json.NewEncoder(metaOut)
		d := map[string]interface{}{
			"name": filepath.Base(p),
			"original": map[string]interface{}{
				"path":   getRelPath(p),
				"height": img.Bounds().Dy(),
				"width":  img.Bounds().Dx(),
			},
			"thumb": map[string]interface{}{
				"path":   getRelPath(thumbPath),
				"height": thumb.Bounds().Dy(),
				"width":  thumb.Bounds().Dx(),
			},
		}

		if err := enc.Encode(d); err != nil {
			return "", fmt.Errorf("%s: %s", metaPath, err)
		}
	}
	return metaPath, nil
}

func thumb(w http.ResponseWriter, r *http.Request) {
	paths, ok := r.URL.Query()["path"]

	if !ok {
		http.Error(w, "Missing path", 400)
		return
	}

	relPath := paths[0]

	p, err := getAbsPath(relPath)

	if err != nil {
		http.Error(w, "Invalid path", 400)
		return
	}

	metaPath, err := makeThumb(p)

	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	http.ServeFile(w, r, metaPath)
}

func get(w http.ResponseWriter, r *http.Request) {
	paths, ok := r.URL.Query()["path"]

	if !ok {
		http.Error(w, "Missing path", 400)
		return
	}

	relPath := paths[0]

	p, err := getAbsPath(relPath)

	if err != nil {
		http.Error(w, "Invalid path", 400)
		return
	}

	http.ServeFile(w, r, p)
}

func download(w http.ResponseWriter, r *http.Request) {
	paths, ok := r.URL.Query()["path"]

	if !ok {
		http.Error(w, "Missing path", 400)
		return
	}

	relPath := paths[0]

	p, err := getAbsPath(relPath)

	if err != nil {
		http.Error(w, "Invalid path", 400)
		return
	}

	w.Header().Set("Content-Disposition", "attachment; filename="+filepath.Base(relPath))

	http.ServeFile(w, r, p)
}

func sendEntries(w http.ResponseWriter, entries []entry) {
	w.Header().Set("Content-Type", "application/json")
	encoder := json.NewEncoder(w)
	err := encoder.Encode(entries)
	if err != nil {
		panic(err)
	}
}

func readEntriesFromPath(p string, entries *[]entry) error {
	file, err := os.Open(p)
	if err != nil {
		return fmt.Errorf("ERROR Failed to open %s: %s", p, err)
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("ERROR Failed to stat %s: %s", p, err)
	}
	if stat.IsDir() {
		names, err := file.Readdirnames(0)
		if err != nil {
			return fmt.Errorf("ERROR Failed to readdir %s: %s", p, err)
		}
		for _, name := range names {
			if err := addEntry(entries, path.Join(p, name)); err != nil {
				return fmt.Errorf("ERROR Failed to read %s: %s", p, err)
			}
		}
	} else {
		if err := addEntry(entries, p); err != nil {
			return fmt.Errorf("ERROR Failed to read %s: %s", p, err)
		}
	}
	return nil
}

func ls(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	paths, ok := q["path"]

	if !ok {
		http.Error(w, "Missing path", 400)
		return
	}

	relPath := paths[0]

	entries := []entry{}

	var p string
	var err error

	if relPath == "/" {
		if !rootIsSingleFolder {
			for _, root := range roots {
				if err := addEntry(&entries, root); err != nil {
					fmt.Printf("ERROR Failed to read %s: %s", root, err)
				}
			}
			sendEntries(w, entries)
			return
		}
		for _, rootPath := range roots {
			p = rootPath
		}
		if err := readEntriesFromPath(p, &entries); err != nil {
			http.Error(w, "Unable to read path", 400)
			return
		}
		sendEntries(w, entries)
		return
	}
	p, err = getAbsPath(relPath)
	if err != nil {
		http.Error(w, "Invalid path", 400)
		return
	}
	if err := readEntriesFromPath(p, &entries); err != nil {
		http.Error(w, "Unable to read path", 400)
		return
	}
	sendEntries(w, entries)
}

type githubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		BrowserDownloadUrl string `json:"browser_download_url"`
	} `json:"assets"`
}

func fetchUi() {
	release := githubRelease{}
	if err := getJson("https://api.github.com/repos/keeth/gopho-ui/releases/latest", &release); err != nil {
		log.Fatal(err)
	}
	zipfile, err := ioutil.TempFile("", "gopho-ui-zip-")
	if err != nil {
		log.Fatal(err)
	}
	downloadFile(zipfile, release.Assets[0].BrowserDownloadUrl)
	zipfile.Close()
	os.RemoveAll(*uiDir)
	unzip(zipfile.Name(), *uiDir)
	os.Remove(zipfile.Name())
	fmt.Printf("gopho-ui %s installed to %s ✓", release.TagName, *uiDir)
}

func indexHtml(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, gophoPath("ui/index.html"))
}

func getRootsJson(filename string) map[string]string {
	raw, err := ioutil.ReadFile(filename)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
	return m
}

var port = flag.Int("port", 3333, "listen port")

func usage() {
	fmt.Fprintln(os.Stderr, "Usages:")
	fmt.Fprintln(os.Stderr, "gopho [OPTIONS] serve (ROOT_DIR|ROOTS_JSON)")
	fmt.Fprintln(os.Stderr, "\tStarts the Gopho server.")
	fmt.Fprintln(os.Stderr, "\tFirst argument can be either:")
	fmt.Fprintln(os.Stderr, "\t\tA folder of images to serve.")
	fmt.Fprintln(os.Stderr, "\t\tA JSON file containing a map of path aliases to folders.")
	fmt.Fprintln(os.Stderr, "gopho [OPTIONS] fetch-ui")
	fmt.Fprintf(os.Stderr, "\tFetch or update the Gopho UI (saved to %s)\n", *uiDir)
	fmt.Fprintln(os.Stderr, "gopho [OPTIONS] thumb (ROOT_DIR|ROOTS_JSON) IMAGE")
	fmt.Fprintf(os.Stderr, "\tGenerate and cache a single image thumbnail and metadata json (saved to %s)\n", *dataDir)
	fmt.Fprintln(os.Stderr, "Options:")
	flag.PrintDefaults()
}

func setRoots(rootArg string) {
	rootFile, err := os.Stat(rootArg)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if rootFile.IsDir() {
		roots = map[string]string{"/" + filepath.Base(rootArg): rootArg}
		rootIsSingleFolder = true
	} else {
		rootsJson := getRootsJson(rootArg)
		for k, v := range rootsJson {
			if strings.HasPrefix(k, "/") {
				roots[k] = v
			} else {
				roots["/"+k] = v
			}
		}
	}

}

func main() {

	flag.Parse()

	args := flag.Args()

	if len(args) > 0 {
		if args[0] == "version" {
			println(version)
			return
		} else if args[0] == "fetch-ui" {
			fetchUi()
			return
		} else if len(args) > 2 && args[0] == "thumb" {
			setRoots(args[1])
			metaPath, err := makeThumb(args[2])
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			fmt.Println(metaPath)
			return
		} else if len(args) > 1 && args[0] == "serve" {
			if _, err := os.Stat(*uiDir); os.IsNotExist(err) {
				fmt.Printf("Gopho UI not found at %s.  I'll try downloading it for you...", *uiDir)
				fetchUi()
			}
			setRoots(args[1])
			ui := http.FileServer(http.Dir(*uiDir))
			mux := goji.NewMux()
			mux.HandleFunc(pat.Get("/ls"), ls)
			mux.HandleFunc(pat.Get("/thumb"), thumb)
			mux.HandleFunc(pat.Get("/get"), get)
			mux.HandleFunc(pat.Get("/download"), download)
			mux.HandleFunc(pat.Get("/p"), indexHtml)
			mux.HandleFunc(pat.Get("/p/*"), indexHtml)
			mux.Handle(pat.Get("/*"), ui)
			mux.Use(cors.Default().Handler)
			fmt.Printf("Listening at http://localhost:%d/", *port)
			err := http.ListenAndServe(fmt.Sprintf("localhost:%d", *port), mux)
			panic(err)
		}
	}
	usage()
}
