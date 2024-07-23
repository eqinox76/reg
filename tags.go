package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"slices"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/distribution/distribution/v3/manifest/schema2"
	"github.com/dustin/go-humanize"
	"github.com/genuinetools/reg/registry"
	"github.com/samber/lo"
)

const tagsHelp = `Get the tags for a repository.`

func (cmd *tagsCommand) Name() string      { return "tags" }
func (cmd *tagsCommand) Args() string      { return "[OPTIONS] NAME[:TAG|@DIGEST]" }
func (cmd *tagsCommand) ShortHelp() string { return tagsHelp }
func (cmd *tagsCommand) LongHelp() string  { return tagsHelp }
func (cmd *tagsCommand) Hidden() bool      { return false }

func (cmd *tagsCommand) Register(fs *flag.FlagSet) {
	fs.BoolVar(&cmd.verbose, "verbose", false, "show more available schema v2 information per tag")
	fs.BoolVar(&cmd.verbose, "v", false, "show more available schema v2 information per tag")
}

type tagsCommand struct {
	verbose bool
}

func (cmd *tagsCommand) Run(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("pass the name of the repository")
	}

	image, err := registry.ParseImage(args[0])
	if err != nil {
		return err
	}

	// Create the registry client.
	r, err := createRegistryClient(ctx, image.Domain)
	if err != nil {
		return err
	}

	tags, err := r.Tags(ctx, image.Path)
	if err != nil {
		return err
	}
	sort.Strings(tags)

	// Print the tags.
	if !cmd.verbose {
		fmt.Println(strings.Join(tags, "\n"))
	} else {
		size := func(m schema2.Manifest) (size int64) {
			for _, l := range m.Layers {
				size += l.Size
			}
			size += m.Config.Size
			return
		}
		type entry struct {
			tag, size, layer, time string
		}
		entries := []entry{}
		for _, tag := range tags {
			m, err := r.ManifestV2(ctx, image.Path, tag)
			if err != nil {
				log.Printf("could not fetch manifest for tag '%s': %s", tag, err)
				continue
			}
			created, _, _ := lo.Must3(r.TagCreatedDate(ctx, image.Path, tag))
			c := ""
			if created != nil {
				c = created.Format(time.RFC3339)
			}
			entries = append(entries, entry{tag: tag, size: humanize.IBytes(uint64(size(m))), layer: m.Layers[len(m.Layers)-1].Digest.String(), time: c})
		}

		slices.SortFunc(entries, func(a, b entry) int {
			if a.time != b.time {
				if a.time == "" {
					return -1
				}
				if b.time == "" {
					return 1
				}
				return -strings.Compare(a.time, b.time)
			}
			return strings.Compare(a.tag, b.tag)
		})

		w := tabwriter.NewWriter(os.Stdout, 1, 1, 1, ' ', 0)
		lo.Must(fmt.Fprintf(w, "tag\tcompressed\tlast layer\tcreated\n"))
		for _, entry := range entries {
			lo.Must(fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", entry.tag, entry.size, entry.layer, entry.time))
		}
		w.Flush()
	}

	return nil
}
