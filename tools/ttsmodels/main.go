// Command ttsmodels extracts the model keys ('Name: options') of the MESBG FTC mod's model bag from a LOCAL Tabletop Simulator saved game.
//
//	go run ./tools/ttsmodels [-guid 24f6d0] [save.json]
//
// Default: the newest ~/Library/Tabletop Simulator/Saves/TS_Save_*.json. Writes only sources/tts/models.json (gitignored: it is the mod's
// content, so it is never committed) as a JSON array: the Nickname of every object in the bag, stripped, duplicates (case and spaces
// folded) removed, in the save's order. Nothing else is read out of the save. The output path is fixed: the tool refuses to write if
// sources/ is not ignored by git. The rules checker and the local-only core test read the file (docs/tabletop-simulator.md).
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/iuya/battle-company-manager-ce-v2/tools/internal/repo"
	"github.com/iuya/battle-company-manager-ce-v2/tools/internal/ttsmodels"
)

func main() {
	guid := flag.String("guid", ttsmodels.BagGUID, "GUID of the model bag")
	flag.Parse()
	if flag.NArg() > 1 {
		fail(fmt.Errorf("at most one saved game"))
	}
	root := repo.Root()
	out := filepath.Join(root, "sources", "tts", "models.json")
	path := flag.Arg(0)
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fail(err)
		}
		if path, err = ttsmodels.NewestSave(filepath.Join(home, "Library", "Tabletop Simulator", "Saves")); err != nil {
			fail(err)
		}
	}
	if err := ttsmodels.CheckOutput(root, out); err != nil {
		fail(err)
	}
	save, err := ttsmodels.ReadSave(path)
	if err != nil {
		fail(err)
	}
	keys, err := ttsmodels.ModelKeys(save, *guid)
	if err != nil {
		fail(err)
	}
	if err := ttsmodels.Write(out, keys); err != nil {
		fail(err)
	}
	fmt.Printf("%d model keys from %s -> sources/tts/models.json (gitignored, never commit it)\n", len(keys), filepath.Base(path))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "ttsmodels:", err)
	os.Exit(1)
}
