package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/hashicorp/go-version"
	"io"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"quay-mirror-version-update/containerApi"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
)

var quayServer = os.Getenv("QUAY_SERVER")
var quayAccessToken = os.Getenv("QUAY_ACCESS_TOKEN")
var quayApi = fmt.Sprintf("https://%s/api/v1/", quayServer)
var quayMirrorNamespace = os.Getenv("QUAY_MIRROR_NAMESPACE")

func contains(arr []string, str string) bool {
	for _, a := range arr {
		if a == str {
			return true
		}
	}
	return false
}

func checkIfImageMightBeMirror(image string) bool {
	return strings.HasPrefix(image, fmt.Sprintf("%s/%s", quayServer, quayMirrorNamespace))
}

func main() {
	var (
		wg     = &sync.WaitGroup{}
		config *rest.Config
		err    error
	)

	if quayMirrorNamespace == "" {
		quayMirrorNamespace = "dockerhub"
	}

	if os.Getenv("MODE") == "out" {
		log.Println("MAIN:  Started in out cluster mode")
		home := homedir.HomeDir()
		config, err = clientcmd.BuildConfigFromFlags("", filepath.Join(home, ".kube", "config"))
		if err != nil {
			log.Fatalln(err.Error())
		}
	} else {
		log.Println("MAIN:  Started in in cluster mode")
		config, err = rest.InClusterConfig()
		if err != nil {
			log.Fatalln(err.Error())
		}
	}

	ignoreNamespacesVar := os.Getenv("IGNORE_NAMESPACES")
	ignoreNamespaces := strings.Split(ignoreNamespacesVar, ",")

	log.Println("MAIN:  Create clientset for configuration")
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalln(err.Error())
	}

	var containers []string
	log.Println("MAIN:  Look for deployments on kubernetes cluster")
	deployments, err := clientset.AppsV1().Deployments(apiv1.NamespaceAll).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		log.Println(err.Error())
	} else {
		log.Printf("MAIN:  Found %d deployments", len(deployments.Items))

		log.Println("MAIN:  Start check for deployment updates")
		for _, deployment := range deployments.Items {
			if contains(ignoreNamespaces, deployment.GetNamespace()) {
				continue
			}

			for _, container := range deployment.Spec.Template.Spec.Containers {
				if !contains(containers, container.Image) && checkIfImageMightBeMirror(container.Image) {
					containers = append(containers, container.Image)
				}
			}
		}
	}

	log.Println("MAIN:  Look for daemon sets on kubernetes cluster")
	daemonSets, err := clientset.AppsV1().DaemonSets(apiv1.NamespaceAll).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		log.Println(err.Error())
	} else {
		log.Printf("MAIN:  Found %d daemon sets", len(daemonSets.Items))

		log.Println("MAIN:  Start check for daemon set updates")
		for _, daemonSet := range daemonSets.Items {
			if contains(ignoreNamespaces, daemonSet.GetNamespace()) {
				continue
			}

			for _, container := range daemonSet.Spec.Template.Spec.Containers {
				if !contains(containers, container.Image) && checkIfImageMightBeMirror(container.Image) {
					containers = append(containers, container.Image)
				}
			}
		}
	}

	log.Println("MAIN:  Look for stateful sets on kubernetes cluster")
	statefulSets, err := clientset.AppsV1().StatefulSets(apiv1.NamespaceAll).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		log.Println(err.Error())
	} else {
		log.Printf("MAIN:  Found %d stateful sets", len(statefulSets.Items))

		log.Println("MAIN:  Start check for stateful set updates")
		for _, statefulSet := range statefulSets.Items {
			if contains(ignoreNamespaces, statefulSet.GetNamespace()) {
				continue
			}

			for _, container := range statefulSet.Spec.Template.Spec.Containers {
				if !contains(containers, container.Image) && checkIfImageMightBeMirror(container.Image) {
					containers = append(containers, container.Image)
				}
			}
		}
	}

	log.Println("MAIN:  Look for cron jobs on kubernetes cluster")
	cronJobs, err := clientset.BatchV1().CronJobs(apiv1.NamespaceAll).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		log.Println(err.Error())
	} else {
		log.Printf("MAIN:  Found %d cron job", len(cronJobs.Items))

		log.Println("MAIN:  Start check for cron job updates")
		for _, cronJob := range cronJobs.Items {
			if contains(ignoreNamespaces, cronJob.GetNamespace()) {
				continue
			}

			for _, container := range cronJob.Spec.JobTemplate.Spec.Template.Spec.Containers {
				if !contains(containers, container.Image) && checkIfImageMightBeMirror(container.Image) {
					containers = append(containers, container.Image)
				}
			}
		}
	}

	log.Printf("MAIN:  Found %d CPUs", runtime.NumCPU())
	wg.Add(runtime.NumCPU())
	containerChan := make(chan string, len(containers))
	logChan := make(chan string)

	for i := 0; i < runtime.NumCPU(); i++ {
		log.Printf("MAIN:  Start process on cpu %d", i)
		go processContainer(wg, containerChan, logChan, i)
	}

	go func(logChan chan string) {
		for logEntry := range logChan {
			log.Println(logEntry)
		}
	}(logChan)

	for _, container := range containers {
		containerChan <- container
	}

	close(containerChan)

	wg.Wait()
	close(logChan)
}

func processContainer(wg *sync.WaitGroup, containerChan chan string, logChan chan string, idx int) {
	for c := range containerChan {
		logf := func(message string, data ...interface{}) {
			logChan <- fmt.Sprintf("CPU "+strconv.Itoa(idx)+": "+message, data...)
		}
		err := updateMirrorTag(c, logf)
		if err != nil {
			logChan <- fmt.Sprintf("CPU %d: Error updating: %s", idx, err.Error())
		}
	}
	logChan <- fmt.Sprintf("CPU %d: Process ended", idx)
	wg.Done()
}

func updateMirrorTag(containerImage string, logf func(message string, data ...interface{})) error {
	logLocal := func(message string, data ...interface{}) {
		logf(fmt.Sprintf("Image: "+containerImage+" "+message, data...))
	}
	type updateMirrorData struct {
		RootRule struct {
			RuleKind  string   `json:"rule_kind"`
			RuleValue []string `json:"rule_value"`
		} `json:"root_rule"`
	}

	type repositoryDetails struct {
		Tags map[string]interface{} `json:"tags"`
	}

	logLocal("Split image")
	imageAndTag := strings.Split(containerImage, ":")
	image := strings.TrimPrefix(imageAndTag[0], quayServer+"/")

	logLocal("Create new details request")
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%srepository/%s?tags=true&stats=false", quayApi, image), nil)
	if err != nil {
		return err
	}

	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", quayAccessToken))

	logLocal("Execute details request")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		var respBuf bytes.Buffer
		_, _ = io.Copy(&respBuf, resp.Body)
		logLocal(respBuf.String())
		return fmt.Errorf("error in details request %s", resp.Status)
	}

	var repoDetails repositoryDetails
	decoder := json.NewDecoder(resp.Body)

	logLocal("Decode detail response")
	err = decoder.Decode(&repoDetails)
	if err != nil {
		return err
	}

	logLocal("Iterate over original tags")
	originalTags := make([]string, len(repoDetails.Tags))
	i := 0
	for key := range repoDetails.Tags {
		originalTags[i] = key
		i++
	}

	dockerImage := strings.ReplaceAll(strings.Split(image, "/")[1], "---", "/")
	tag, err := containerApi.GetLatestTag(dockerImage, logLocal)
	if err != nil {
		return err
	}

	logLocal("Split tag in tag and suffix")
	splitTag := strings.Split(imageAndTag[1], "-")
	if len(splitTag) > 1 && !strings.HasSuffix(tag, "-"+splitTag[1]) {
		tag = tag + "-" + splitTag[1]
	}

	keepOnlyLastTwoVersions := os.Getenv("ONLY_KEEP_TWO_LAST_VERSIONS") == "True" || os.Getenv("ONLY_KEEP_TWO_LAST_VERSIONS") == "true"
	if keepOnlyLastTwoVersions {
		targetTags := make([]string, 0)

		versions := make([]*version.Version, 0)
		for _, raw := range originalTags {
			v, _ := version.NewVersion(raw)
			if v != nil {
				versions = append(versions, v)
			} else {
				targetTags = append(targetTags, raw)
			}
		}

		sort.Sort(sort.Reverse(version.Collection(versions)))
		targetTags = append(targetTags, tag)
		targetTags = append(targetTags, versions[0].Original())
	} else {
		if !contains(originalTags, tag) {
			originalTags = append(originalTags, tag)
		} else {
			return nil
		}
	}

	updateMirrorPayload := updateMirrorData{
		RootRule: struct {
			RuleKind  string   `json:"rule_kind"`
			RuleValue []string `json:"rule_value"`
		}{
			RuleKind:  "tag_glob_csv",
			RuleValue: originalTags,
		},
	}

	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	logLocal("Encode update mirror payload")
	err = encoder.Encode(updateMirrorPayload)
	if err != nil {
		return err
	}

	logLocal("Create request to update mirror")
	req, err = http.NewRequest(http.MethodPut, fmt.Sprintf("%srepository/%s/mirror", quayApi, image), &encoded)
	if err != nil {
		return err
	}

	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", quayAccessToken))
	req.Header.Add("Content-Type", "application/json")

	logLocal("Execute request to update mirror")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusCreated {
		var respBuf bytes.Buffer
		_, _ = io.Copy(&respBuf, resp.Body)
		logLocal(respBuf.String())
		return fmt.Errorf("error in update request %s", resp.Status)
	}

	logLocal("Create mirror sync request")
	req, err = http.NewRequest(http.MethodPost, fmt.Sprintf("%srepository/%s/mirror/sync-now", quayApi, image), nil)
	if err != nil {
		return err
	}

	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", quayAccessToken))
	logLocal("Execute mirror sync request")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusNoContent {
		var respBuf bytes.Buffer
		_, _ = io.Copy(&respBuf, resp.Body)
		logLocal(respBuf.String())
		return fmt.Errorf("error in sync request %s", resp.Status)
	}

	return nil
}
