package main

import (
	"embed"
	"flag"
	"html/template"
	"io/fs"
	"log"
	"net/http"
)

//go:embed templates/*.tmpl
var templatesFS embed.FS

//go:embed static
var embeddedStaticFS embed.FS

// staticFS strips the "static/" prefix so http.FileServer serves
// static/style.css as /static/style.css rather than /static/static/style.css.
var staticFS fs.FS

func main() {
	addr := flag.String("addr", ":8080", "address to listen on")
	flag.Parse()

	var err error
	staticFS, err = fs.Sub(embeddedStaticFS, "static")
	if err != nil {
		log.Fatalf("static assets: %v", err)
	}

	tmpl, err := template.ParseFS(templatesFS, "templates/*.tmpl")
	if err != nil {
		log.Fatalf("parsing templates: %v", err)
	}

	srv := NewServer(NewSessionStore(), tmpl)

	log.Printf("listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, srv.routes()))
}
