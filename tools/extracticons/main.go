// tools/extracticons/main.go
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/png"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"skyverseweb/iconextract"
)

func main() {
	paksDir := flag.String("paks", "/home/binarybird/.local/share/Steam/steamapps/common/Everwind/skyverse/Content/Paks", "game's Paks directory")
	retocPath := flag.String("retoc", "tools/extracticons/retoc", "path to the retoc binary")
	itemsPath := flag.String("items", "static/items.json", "existing item catalog (3-field format) to read and rewrite (4-field)")
	iconsOut := flag.String("icons-out", "static/icons", "directory to write extracted icon PNGs into")
	workDir := flag.String("work", "/tmp/extracticons-work", "scratch directory for retoc's legacy conversion output")
	flag.Parse()

	if err := run(*paksDir, *retocPath, *itemsPath, *iconsOut, *workDir); err != nil {
		log.Fatal(err)
	}
}

type itemEntry struct {
	Name       string
	Category   string
	ObjectPath string
}

func run(paksDir, retocPath, itemsPath, iconsOut, workDir string) error {
	items, err := readItems(itemsPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", itemsPath, err)
	}
	log.Printf("loaded %d items from %s", len(items), itemsPath)

	itemsLegacyDir := filepath.Join(workDir, "items")
	iconsLegacyDir := filepath.Join(workDir, "icons")
	if err := retocToLegacy(retocPath, "Skyverse/Content/Data/Items/", paksDir, itemsLegacyDir); err != nil {
		return fmt.Errorf("bulk-converting item assets: %w", err)
	}
	if err := retocToLegacy(retocPath, "Skyverse/Content/Textures/Icons/", paksDir, iconsLegacyDir); err != nil {
		return fmt.Errorf("bulk-converting icon assets: %w", err)
	}

	iconRefPattern := regexp.MustCompile(`/Game/Textures/Icons/[^\x00]*`)
	if err := os.MkdirAll(iconsOut, 0o755); err != nil {
		return err
	}

	found, missing := 0, 0
	for _, it := range items {
		// The icon reference (an object path string) lives in the item
		// asset's .uasset (name/import table), not its .uexp (export
		// property data) -- verified empirically against the real
		// converted assets before writing this loop.
		itemUasset := legacyUassetPath(itemsLegacyDir, it.ObjectPath)
		data, err := os.ReadFile(itemUasset)
		if err != nil {
			log.Printf("skip %s: item asset not converted (%v)", it.ObjectPath, err)
			missing++
			continue
		}
		match := iconRefPattern.Find(data)
		if match == nil {
			log.Printf("skip %s: no icon reference found in its asset", it.ObjectPath)
			missing++
			continue
		}
		iconObjectPath := string(match)
		iconUexp := legacyUexpPath(iconsLegacyDir, iconObjectPath)
		iconData, err := os.ReadFile(iconUexp)
		if err != nil {
			log.Printf("skip %s: referenced icon %s not converted (%v)", it.ObjectPath, iconObjectPath, err)
			missing++
			continue
		}
		img, err := iconextract.Decode(iconData)
		if err != nil {
			log.Printf("skip %s: decoding icon %s: %v", it.ObjectPath, iconObjectPath, err)
			missing++
			continue
		}
		iconFile := slugify(it.Name) + ".png"
		if err := writePNG(filepath.Join(iconsOut, iconFile), img); err != nil {
			return fmt.Errorf("writing %s: %w", iconFile, err)
		}
		found++
	}
	log.Printf("extracted %d icons, %d items had no usable icon", found, missing)

	if err := writePlaceholder(filepath.Join(iconsOut, "_placeholder.png")); err != nil {
		return fmt.Errorf("writing placeholder icon: %w", err)
	}

	return writeItemsWithIcons(itemsPath, items, iconsOut)
}

func readItems(path string) ([]itemEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw [][]string
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	items := make([]itemEntry, 0, len(raw))
	for _, e := range raw {
		if len(e) < 3 {
			continue
		}
		items = append(items, itemEntry{Name: e[0], Category: e[1], ObjectPath: e[2]})
	}
	return items, nil
}

// legacyUexpPath maps a gvas object path ("/Game/Foo/Bar.Bar") to the
// file retoc to-legacy writes it to ("<root>/Skyverse/Content/Foo/Bar.uexp").
func legacyUexpPath(legacyRoot, objectPath string) string {
	packagePath := strings.SplitN(objectPath, ".", 2)[0]
	rel := "Skyverse/Content" + strings.TrimPrefix(packagePath, "/Game") + ".uexp"
	return filepath.Join(legacyRoot, rel)
}

// legacyUassetPath is legacyUexpPath's counterpart for the .uasset half
// of the pair -- the name/import table that holds referenced object
// path strings (like an icon reference), as opposed to .uexp's export
// property data.
func legacyUassetPath(legacyRoot, objectPath string) string {
	packagePath := strings.SplitN(objectPath, ".", 2)[0]
	rel := "Skyverse/Content" + strings.TrimPrefix(packagePath, "/Game") + ".uasset"
	return filepath.Join(legacyRoot, rel)
}

func retocToLegacy(retocPath, filter, paksDir, outDir string) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	cmd := exec.Command(retocPath, "to-legacy", "-f", filter, paksDir, outDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

var slugInvalid = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

func slugify(name string) string {
	return slugInvalid.ReplaceAllString(name, "_")
}

func writePNG(path string, img *image.NRGBA) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// writePlaceholder writes a small flat gray-with-alpha square, used for
// any item with no extracted icon.
func writePlaceholder(path string) error {
	return writePNG(path, newPlaceholder(32))
}

// newPlaceholder builds a plain dim x dim semi-transparent gray square.
func newPlaceholder(dim int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, dim, dim))
	for i := 0; i < dim*dim; i++ {
		img.Pix[i*4], img.Pix[i*4+1], img.Pix[i*4+2], img.Pix[i*4+3] = 128, 128, 128, 180
	}
	return img
}

func writeItemsWithIcons(path string, items []itemEntry, iconsOut string) error {
	out := make([][4]string, len(items))
	for i, it := range items {
		iconFile := ""
		candidate := filepath.Join(iconsOut, slugify(it.Name)+".png")
		if _, err := os.Stat(candidate); err == nil {
			iconFile = slugify(it.Name) + ".png"
		}
		out[i] = [4]string{it.Name, it.Category, it.ObjectPath, iconFile}
	}
	data, err := json.Marshal(out)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
