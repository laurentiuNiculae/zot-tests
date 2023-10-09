package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	ispec "github.com/opencontainers/image-spec/specs-go/v1"
	"gopkg.in/resty.v1"

	"zotregistry.io/zot/pkg/api/constants"
	. "zotregistry.io/zot/pkg/test/image-utils"
)

var (
	ZotURL   = "http://127.0.0.1:5000" // Connection info
	User     = "laur"
	Password = "password"
)

func main() {
	resty.DefaultClient.DisableWarn = true

	if len(os.Args) == 1 {
		fmt.Println("Specify the port and optionally the user/password for basic auth.\n" +
			"The `--no-gen-data` flag will stop pushing test images to the server before starting the requests")
		fmt.Println()
		fmt.Println("Example: $ zot-tests <port> [user, password] [--no-gen-data]")
		os.Exit(0)
	}

	ZotURL = fmt.Sprintf("http://127.0.0.1:%s", GetPort())
	User, Password = GetUserPass()

	if !NoGen() {
		GenerateAndUploadTestData()
	}

	for i := 0; true; i++ {
		RequestHomePage()

		if i%10 == 0 {
			fmt.Printf("Home Requests: %d\n", i)
		}
	}
}

func GetPort() string {
	if len(os.Args) >= 1+1 {
		return os.Args[1]
	}

	return "5000"
}

func GetUserPass() (string, string) {
	if len(os.Args) >= 1+3 {
		return os.Args[2], os.Args[3]
	}

	return "", ""
}

func NoGen() bool {
	for _, arg := range os.Args {
		if strings.Contains(arg, "no-gen-data") {
			return true
		}
	}

	return false
}

func GenerateAndUploadTestData() {
	repoCount := 100
	manifestImageCount := 10
	multiarchCount := 5

	for repoId := 0; repoId < repoCount; repoId++ {
		for i := 0; i < manifestImageCount; i++ {
			err := UploadImageWithBasicAuth(
				CreateImageWith().
					RandomLayers(2, 5).
					RandomConfig().
					Annotations(map[string]string{
						ispec.AnnotationDescription: fmt.Sprintf("Description %d", repoId),
						ispec.AnnotationAuthors:     fmt.Sprintf("Authors %d", repoId),
						ispec.AnnotationVendor:      fmt.Sprintf("Vendor %d", repoId),
						ispec.AnnotationTitle:       fmt.Sprintf("Title %d", repoId),
					}).Build(),
				ZotURL,
				fmt.Sprintf("repo%d", repoId),
				fmt.Sprintf("tag%d", i),
				User, Password,
			)
			if err != nil {
				panic(err)
			}

			fmt.Printf("Repo: %d, Manifest Tag: %d\n", repoId, i)
		}

		for i := 0; i < multiarchCount; i++ {
			err := UploadMultiarchImageWithBasicAuth(
				CreateRandomMultiarch(),
				ZotURL,
				fmt.Sprintf("repo%d", repoId),
				fmt.Sprintf("tag-multi%d", i),
				User, Password,
			)
			if err != nil {
				panic(err)
			}

			fmt.Printf("Repo: %d, Index Tag: %d\n", repoId, i)
		}
	}
}

const (
	homeQuery1 = `{GlobalSearch(query:"", requestedPage: {limit:3 offset:0 sortBy: DOWNLOADS} ) {Page {TotalCount ItemCount} Repos {Name LastUpdated Size Platforms { Os Arch } IsStarred IsBookmarked NewestImage { Tag Vulnerabilities {MaxSeverity Count} Description IsSigned SignatureInfo { Tool IsTrusted Author } Licenses Vendor Labels } DownloadCount}}}`
	homeQuery2 = `{GlobalSearch(query:"", requestedPage: {limit:2 offset:0 sortBy: UPDATE_TIME} ) {Page {TotalCount ItemCount} Repos {Name LastUpdated Size Platforms { Os Arch } IsStarred IsBookmarked NewestImage { Tag Vulnerabilities {MaxSeverity Count} Description IsSigned SignatureInfo { Tool IsTrusted Author } Licenses Vendor Labels } DownloadCount}}}`
	homeQuery3 = `{GlobalSearch(query:"", requestedPage: {limit:2 offset:0 sortBy: RELEVANCE} ,filter: { IsBookmarked: true}) {Page {TotalCount ItemCount} Repos {Name LastUpdated Size Platforms { Os Arch } IsStarred IsBookmarked NewestImage { Tag Vulnerabilities {MaxSeverity Count} Description IsSigned SignatureInfo { Tool IsTrusted Author } Licenses Vendor Labels } DownloadCount}}}`
)

func RequestHomePage() {
	wg := &sync.WaitGroup{}

	wg.Add(3)
	go RunQuery(homeQuery1, wg)
	go RunQuery(homeQuery2, wg)
	go RunQuery(homeQuery3, wg)

	wg.Wait()
}

func RunQuery(query string, wg *sync.WaitGroup) {
	resp, err := resty.R().SetBasicAuth(User, Password).Get("http://127.0.0.1:5000" + constants.FullSearchPrefix + "?query=" + url.QueryEscape(query))
	if err != nil || resp.StatusCode() != 200 {
		panic(fmt.Errorf("StatusCode: %d Err: %w", resp.StatusCode(), err))
	}

	wg.Done()
}

func UploadMultiarchImageWithBasicAuth(multiImage MultiarchImage, baseURL string, repo, ref, user, password string) error {
	for _, image := range multiImage.Images {
		err := UploadImageWithBasicAuth(image, baseURL, repo, image.DigestStr(), user, password)
		if err != nil {
			return err
		}
	}

	// put manifest
	indexBlob, err := json.Marshal(multiImage.Index)
	if err != nil {
		return err
	}

	resp, err := resty.R().SetBasicAuth(user, password).
		SetHeader("Content-type", ispec.MediaTypeImageIndex).
		SetBody(indexBlob).
		Put(baseURL + "/v2/" + repo + "/manifests/" + ref)

	if resp.StatusCode() != http.StatusCreated {
		return ErrPutIndex
	}

	return err
}
