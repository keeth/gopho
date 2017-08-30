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

	"github.com/nfnt/resize"
	"goji.io"
	"goji.io/pat"
)

var roots = []string{
	"/Users/keith/Downloads/test/My Bio",
}

var thumbsFolder = "/Users/keith/.gopho/thumbs"

type entry struct {
	Name    string
	Path    string
	Size    int64
	IsDir   bool
	IsImage bool
	Thumb   string
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
	if err := os.MkdirAll(thumbsFolder, 0755); err != nil {
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
			Path:  p,
			Size:  stat.Size(),
			IsDir: true,
		})
		return nil
	}
	if imageRe.MatchString(p) {
		hash := md5String(p)
		thumbPath := path.Join(thumbsFolder, hash+".jpg")
		if _, err := os.Stat(thumbPath); os.IsNotExist(err) {
			var img image.Image
			if pngRe.MatchString(p) {
				img, err = png.Decode(file)
			} else {
				img, err = jpeg.Decode(file)
			}
			if err != nil {
				return err
			}
			thumb := resize.Thumbnail(1200, 1200, img, resize.Bicubic)
			out, err := os.Create(thumbPath)
			if err != nil {
				return err
			}
			defer out.Close()
			jpeg.Encode(out, thumb, nil)
		}

		*entries = append(*entries, entry{
			Name:    stat.Name(),
			Path:    p,
			Size:    stat.Size(),
			IsImage: true,
			Thumb:   thumbPath,
		})
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

	requestedPath := paths[0]

	entries := []entry{}

	if requestedPath == "/" {
		for _, root := range roots {
			if err := addEntry(&entries, root); err != nil {
				log.Printf("ERROR Failed to read %s: %s", requestedPath, err)
			}
		}
	} else {
		file, err := os.Open(requestedPath)
		if err != nil {
			log.Printf("ERROR Failed to open %s: %s", requestedPath, err)
			http.Error(w, "Failed to open path", 400)
			return
		}
		defer file.Close()
		stat, err := file.Stat()
		if err != nil {
			log.Printf("ERROR Failed to stat %s: %s", requestedPath, err)
			http.Error(w, "Failed to stat path", 400)
			return
		}
		if stat.IsDir() {
			names, err := file.Readdirnames(0)
			if err != nil {
				log.Printf("ERROR Failed to readdir %s: %s", requestedPath, err)
				http.Error(w, "Failed to readdir path", 400)
				return
			}
			for _, name := range names {
				if err := addEntry(&entries, path.Join(requestedPath, name)); err != nil {
					log.Printf("ERROR Failed to read %s: %s", requestedPath, err)
					return
				}
			}
		} else {
			if err := addEntry(&entries, requestedPath); err != nil {
				log.Printf("ERROR Failed to read %s: %s", requestedPath, err)
				return
			}
		}
	}

	encoder := json.NewEncoder(w)
	err := encoder.Encode(entries)
	if err != nil {
		panic(err)
	}
}

func main() {
	mux := goji.NewMux()
	mux.HandleFunc(pat.Get("/ls"), ls)
	print("Running on http://localhost:3333/")
	err := http.ListenAndServe("localhost:3333", mux)
	panic(err)
}
