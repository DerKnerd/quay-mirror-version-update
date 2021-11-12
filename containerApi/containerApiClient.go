package containerApi

import (
	"encoding/json"
	"fmt"
	"github.com/hashicorp/go-version"
	"io/ioutil"
	"net/http"
	"sort"
)

type authentication struct {
	Token string `json:"token"`
}

type TagList struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

func GetLatestTag(image string, logf func(message string, data ...interface{})) (string, error) {
	logf("Get latest image version from docker hub")
	logf("Run authentication request")
	authResponse, err := http.DefaultClient.Get("https://auth.docker.io/token?service=registry.docker.io&scope=repository:" + image + ":pull")
	if err != nil {
		return "", err
	}
	if authResponse.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to login")
	}

	auth := new(authentication)
	body, err := ioutil.ReadAll(authResponse.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read body")
	}

	err = json.Unmarshal(body, auth)
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal body")
	}

	logf("Got api token")
	logf("Get all tags for image %s", image)
	req, err := http.NewRequest(http.MethodGet, "https://registry-1.docker.io/v2/"+image+"/tags/list", nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request")
	}

	req.Header.Set("Authorization", "Bearer "+auth.Token)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get tags")
	}

	body, err = ioutil.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read body")
	}

	var tags TagList
	err = json.Unmarshal(body, &tags)

	if err != nil {
		return "", fmt.Errorf("failed to unmarshal body %s", err.Error())
	}

	versions := make([]*version.Version, 0)
	for _, raw := range tags.Tags {
		v, _ := version.NewVersion(raw)
		if v != nil {
			versions = append(versions, v)
		}
	}

	sort.Sort(sort.Reverse(version.Collection(versions)))

	return versions[0].String(), nil
}
