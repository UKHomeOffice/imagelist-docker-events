package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/urfave/cli"
)

const (
	imageListPutImagesPath = "/images"
)

var (
	tagRegexp = regexp.MustCompile(`:([\w][\w.-]{0,127})$`)
	logger    = log.New(os.Stderr, "", log.Ldate|log.Ltime|log.Lshortfile)
)

func main() {
	app := cli.NewApp()
	app.Name = "imagelist-docker-events"
	app.Version = "v0.0.1"
	app.Usage = "poll docker for image push events and add them to imagelist service"

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "imagelist-url",
			Usage: "imagelist service url",
		},
	}

	app.Action = func(c *cli.Context) error {
		f := filters.NewArgs()
		f.Add("type", events.ImageEventType)

		if !c.IsSet("imagelist-url") {
			return cli.NewExitError("error: imagelist-url needs to be set", 1)
		}
		u := c.String("imagelist-url")
		imageListURL, err := joinURL(u, imageListPutImagesPath)
		if err != nil {
			return cli.NewExitError(fmt.Sprintf("error: failed to parse imagelist url: %v", err), 1)
		}

		// main loop
		for {
			c, err := client.NewEnvClient()
			if err != nil {
				logger.Printf("error: docker connection failed: %v", err)
			}
			defer c.Close()

			messages, errs := c.Events(context.Background(), types.EventsOptions{Filters: f})
			if err := processEvents(messages, errs, imageListURL); err != nil {
				logger.Print(err)
				<-time.After(time.Second)
				continue
			}
		}

	}

	app.Run(os.Args)
}

func processEvents(messages <-chan events.Message, errs <-chan error, url string) error {
	select {
	case err := <-errs:
		if err != nil && err != io.EOF {
			return fmt.Errorf("error: failed to read events: %v", err)
		}
	case m := <-messages:
		if m.Type == "image" && m.Action == "push" {
			go addToImageList(m.ID, url)
		}
	}
	return nil
}

type image struct {
	ID   string   `json:"id"`
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

func addToImageList(name string, url string) {
	images, err := getRepoDigests(name)
	if err != nil {
		logger.Print(err)
		return
	}

	for _, i := range images {
		go func(i image) {
			attempt := 0
			for {
				if attempt == 30 {
					logger.Printf("error submitting %s image: max retries reached", name)
					return
				}

				if attempt != 0 {
					<-time.After(time.Second * 3)
				}
				attempt++

				data, err := json.Marshal(i)
				if err != nil {
					logger.Printf("error submitting %q image: %v", i.Name, err)
					return
				}

				resp, err := httpPut(url, "application/json", bytes.NewReader(data))
				if err != nil {
					logger.Printf("error submitting %q image: %v", i.Name, err)
					continue
				}
				defer func() {
					io.Copy(ioutil.Discard, resp.Body)
					resp.Body.Close()
				}()

				if resp.StatusCode == http.StatusOK {
					logger.Printf("submitted %+v", i)
					return
				}

				if resp.StatusCode == http.StatusInternalServerError {
					logger.Printf("error submitting %q image: got http status code %d from imagelist", name, resp.StatusCode)
					continue
				} else {
					logger.Printf("error submitting %q image: bad request, status %d from imagelist", name, resp.StatusCode)
					return
				}
			}
		}(i)
	}
}

func getRepoDigests(name string) ([]image, error) {
	var images []image

	// Could use a global client, but docker client does not implement
	// auto-reconnect and I don't want to implement this myself.
	c, err := client.NewEnvClient()
	if err != nil {
		return images, err
	}
	defer c.Close()

	if !tagRegexp.MatchString(name) {
		return images, fmt.Errorf("unable to find image %q without a tag", name)
	}

	ctx := context.Background()
	imageInspect, _, err := c.ImageInspectWithRaw(ctx, name)
	if err != nil {
		return images, err
	}

	dm := mapRepoDigestsToTags(name, imageInspect)
	if len(dm) == 0 {
		return images, fmt.Errorf("unable to find repo digests for %q image", name)
	}

	for k, v := range mapRepoDigestsToTags(name, imageInspect) {
		images = append(images, image{k, tagRegexp.ReplaceAllString(name, ""), v})
	}

	return images, nil
}

// mapRepoDigestsToTags finds RepoDigests that match image name and returns a
// map of repoDigest to tags.
func mapRepoDigestsToTags(name string, image types.ImageInspect) map[string][]string {
	m := make(map[string][]string)
	if name == "" {
		return m
	}

	// trim a tag from image name if exists
	name = tagRegexp.ReplaceAllString(name, "")

	tags := []string{}
	for _, entry := range image.RepoTags {
		if strings.HasPrefix(entry, name) {
			matches := tagRegexp.FindStringSubmatch(entry)
			if len(matches) == 2 {
				tags = append(tags, matches[1])
			}
		}
	}

	for _, entry := range image.RepoDigests {
		if strings.HasPrefix(entry, name) {
			m[entry] = tags
		}
	}

	return m
}

func httpPut(url string, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPut, url, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", contentType)
	c := &http.Client{}
	return c.Do(req)
}

func joinURL(u, path string) (string, error) {
	p, err := url.Parse(path)
	if err != nil {
		return "", err
	}
	base, err := url.Parse(u)
	if err != nil {
		return "", err
	}

	return base.ResolveReference(p).String(), nil
}
