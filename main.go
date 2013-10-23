package main

import (
	"crypto/sha1"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/peterbourgon/diskv"
	"html/template"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
)

var (
	templates = template.Must(template.ParseFiles(
		"html/new_image.html",
		"html/annotate.html",
		"html/view.html",
	))
	db *diskv.Diskv
)

func fileSHA(data []byte) string {
	sha := sha1.New()
	sha.Write(data)
	return fmt.Sprintf("%x", string(sha.Sum(nil))[0:8])
}

func NewImage(w http.ResponseWriter, req *http.Request) {
	if err := templates.ExecuteTemplate(w, "new_image.html", nil); err != nil {
		log.Printf("Execute template error: %v", err)
	}
}

func PostImage(w http.ResponseWriter, req *http.Request) {
	file, _, err := req.FormFile("image")
	if err != nil {
		log.Printf("FormFile error: %v", err)
		http.Error(w, "Sorry, something broke!", 500)
		return
	}

	var data []byte
	data, err = ioutil.ReadAll(file)
	if err != nil {
		log.Printf("ReadAll error: %v", err)
		http.Error(w, "Sorry, something broke!", 500)
		return
	}

	sha := fileSHA(data)
	err = ioutil.WriteFile("images/"+sha, data, 0777)
	if err != nil {
		log.Printf("WriteFile error: %v", err)
		http.Error(w, "Sorry, something broke!", 500)
		return
	}

	log.Printf("%s uploaded", sha)

	http.Redirect(w, req, "/view/"+sha, 303)
}

func Annotate(w http.ResponseWriter, req *http.Request) {
	image := req.URL.Path[len("/annotate/"):]
	if len(image) == 0 {
		http.Error(w, "No image specified!", 400)
		return
	}

	if req.Method == "GET" {
		if _, err := db.Read(image); err == nil {
			log.Printf("% already exists, redirecting", image)
			http.Redirect(w, req, "/view/"+image, 303)
			return
		}
		templates.ExecuteTemplate(w, "annotate.html", struct{ Image string }{image})
	} else if req.Method == "POST" {
		if err := req.ParseForm(); err != nil {
			log.Println("Bad form submitted")
			http.Error(w, "Bad form!", 400)
			return
		}
		picks, ok := req.PostForm["pick"]
		if !ok {
			log.Println("Can't find pick in form")
			http.Error(w, "Bad form!", 400)
			return
		}

		encoded, err := json.Marshal(picks)
		if err != nil {
			log.Printf("Can't encode picks: %v", picks)
			http.Error(w, "Sorry, something broke!", 400)
			return
		}

		if err := db.Write(image, encoded); err != nil {
			log.Printf("Problem writing to db: %s: %s", image, encoded)
			http.Error(w, "Sorry, something broke!", 500)
			return
		}

		log.Printf("%s picked: %v", image, picks)

		http.Redirect(w, req, "/view/"+image, 303)
	}
}

func ViewImage(w http.ResponseWriter, req *http.Request) {
	image := req.URL.Path[len("/view/"):]
	if len(image) == 0 {
		http.Error(w, "No image specified!", 400)
		return
	}

	type Coordinate struct {
		X int
		Y int
	}

	type Data struct {
		Image       string
		Coordinates []Coordinate
		Annotated   bool
	}

	data := Data{
		Image:       image,
		Coordinates: []Coordinate{},
		Annotated:   false,
	}

	if read, err := db.Read(image); err == nil {
		var picks []string
		if err := json.Unmarshal(read, &picks); err != nil {
			log.Printf("Broke in the unmarshal: %s: %v", image, err)
			http.Error(w, "Sorry, something broke!", 500)
			return
		}

		for _, p := range picks {
			s := strings.Split(p, ",")
			x, _ := strconv.Atoi(s[0])
			x -= 50
			y, _ := strconv.Atoi(s[1])
			y -= 50

			data.Coordinates = append(data.Coordinates, Coordinate{X: x, Y: y})
		}

		data.Annotated = true
	}

	log.Println(data)

	templates.ExecuteTemplate(w, "view.html", data)
}

func Hello(w http.ResponseWriter, req *http.Request) {
	fmt.Fprintf(w, "Hi, you found selector")
}

func main() {
	port := flag.String("port", "7777", "port to listen on")

	flag.Parse()

	http.HandleFunc("/post_images", PostImage)
	http.HandleFunc("/annotate/", Annotate)
	http.HandleFunc("/view/", ViewImage)
	http.HandleFunc("/new", NewImage)
	http.Handle("/images/", http.StripPrefix("/images/", http.FileServer(http.Dir("images"))))
	http.HandleFunc("/", Hello)

	flatTransform := func(s string) []string { return []string{} }
	db = diskv.New(diskv.Options{
		BasePath:  "data",
		Transform: flatTransform,
	})

	hp := net.JoinHostPort("", *port)
	if err := http.ListenAndServe(hp, nil); err != nil {
		panic(err)
	}
}
