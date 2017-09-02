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

	"github.com/nfnt/resize"
	"github.com/rs/cors"
	"goji.io"
	"goji.io/pat"
)

func gophoPath(p string) string {
	usr, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}
	return path.Join(usr.HomeDir, ".gopho", p)
}

var roots = map[string]string{
	"/My Bio": "/Users/keith/Downloads/test/My Bio",
}

var dataDir = "/Users/keith/.gopho/data"

var dataPath = "/data"

type entry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	IsDir   bool   `json:"isDir"`
	IsImage bool   `json:"isImage"`
}

var imageRe, pngRe *regexp.Regexp

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
	if err := os.MkdirAll(dataDir, 0755); err != nil {
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
		return strings.Replace(p, dataPath, dataDir, 1), nil
	}
	for rel, abs := range roots {
		if strings.HasPrefix(p, rel) {
			return strings.Replace(p, rel, abs, 1), nil
		}
	}
	return "", errors.New("Invalid path")
}

func getRelPath(p string) string {
	if strings.HasPrefix(p, dataDir) {
		return strings.Replace(p, dataDir, dataPath, 1)
	}
	for rel, abs := range roots {
		if strings.HasPrefix(p, abs) {
			return strings.Replace(p, abs, rel, 1)
		}
	}
	log.Panicf("Invalid path %s", p)
	return ""
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

	file, err := os.Open(p)
	if err != nil {
		log.Printf("ERROR Failed to open %s: %s", p, err)
		http.Error(w, "Failed to open file", 400)
		return
	}
	defer file.Close()

	hash := md5String(p)
	thumbPath := path.Join(dataDir, hash+".jpg")
	metaPath := path.Join(dataDir, hash+".json")
	if _, err := os.Stat(thumbPath); os.IsNotExist(err) {
		var img image.Image
		if pngRe.MatchString(p) {
			img, err = png.Decode(file)
		} else {
			img, err = jpeg.Decode(file)
		}
		if err != nil {
			log.Printf("ERROR Failed to decode %s: %s", p, err)
			http.Error(w, "Failed to decode file", 400)
			return
		}
		thumb := resize.Thumbnail(1200, 1200, img, resize.Bicubic)
		thumbOut, err := os.Create(thumbPath)
		if err != nil {
			log.Printf("ERROR Failed to create thumb file %s: %s", thumbPath, err)
			http.Error(w, "Failed to create file", 400)
			return
		}
		defer thumbOut.Close()
		jpeg.Encode(thumbOut, thumb, nil)

		metaOut, err := os.Create(metaPath)
		if err != nil {
			log.Printf("ERROR Failed to create meta file %s: %s", thumbPath, err)
			http.Error(w, "Failed to create file", 400)
			return
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
			log.Printf("ERROR Failed to write meta file %s: %s", thumbPath, err)
			http.Error(w, "Failed to write file", 400)
			return
		}
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

func ls(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	paths, ok := q["path"]

	if !ok {
		http.Error(w, "Missing path", 400)
		return
	}

	relPath := paths[0]

	entries := []entry{}

	if relPath == "/" {
		for _, root := range roots {
			if err := addEntry(&entries, root); err != nil {
				log.Printf("ERROR Failed to read %s: %s", root, err)
			}
		}
	} else {
		p, err := getAbsPath(relPath)

		if err != nil {
			http.Error(w, "Invalid path", 400)
			return
		}

		file, err := os.Open(p)
		if err != nil {
			log.Printf("ERROR Failed to open %s: %s", p, err)
			http.Error(w, "Failed to open path", 400)
			return
		}
		defer file.Close()
		stat, err := file.Stat()
		if err != nil {
			log.Printf("ERROR Failed to stat %s: %s", p, err)
			http.Error(w, "Failed to stat path", 400)
			return
		}
		if stat.IsDir() {
			names, err := file.Readdirnames(0)
			if err != nil {
				log.Printf("ERROR Failed to readdir %s: %s", p, err)
				http.Error(w, "Failed to readdir path", 400)
				return
			}
			for _, name := range names {
				if err := addEntry(&entries, path.Join(p, name)); err != nil {
					log.Printf("ERROR Failed to read %s: %s", p, err)
					return
				}
			}
		} else {
			if err := addEntry(&entries, p); err != nil {
				log.Printf("ERROR Failed to read %s: %s", p, err)
				return
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	encoder := json.NewEncoder(w)
	err := encoder.Encode(entries)
	if err != nil {
		panic(err)
	}
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
	zipfile := gophoPath("ui.zip")
	uidir := gophoPath("ui")
	downloadFile(zipfile, release.Assets[0].BrowserDownloadUrl)
	os.RemoveAll(uidir)
	unzip(zipfile, uidir)
	os.Remove(zipfile)
	fmt.Printf("gopho-ui %s installed to %s ✓", release.TagName, uidir)
}

func indexHtml(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, gophoPath("ui/index.html"))
}

func main() {

	if len(os.Args) > 1 {
		if os.Args[1] == "fetch-ui" {
			fetchUi()
			return
		}
	}
	ui := http.FileServer(http.Dir(gophoPath("ui")))
	mux := goji.NewMux()
	mux.HandleFunc(pat.Get("/ls"), ls)
	mux.HandleFunc(pat.Get("/thumb"), thumb)
	mux.HandleFunc(pat.Get("/get"), get)
	mux.HandleFunc(pat.Get("/download"), download)
	mux.HandleFunc(pat.Get("/p"), indexHtml)
	mux.HandleFunc(pat.Get("/p/*"), indexHtml)
	mux.Handle(pat.Get("/*"), ui)
	mux.Use(cors.Default().Handler)
	print("Running on http://localhost:3333/")
	err := http.ListenAndServe("localhost:3333", mux)
	panic(err)
}
