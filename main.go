package main

// Copyright (c) 2018 Bhojpur Consulting Private Limited, India. All rights reserved.

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:

// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"sync"
	"time"

	cmdhelpers "github.com/bhojpur/iso/cmd/manager/helpers"
	"github.com/bhojpur/iso/pkg/manager/api/client"
	bisoClient "github.com/bhojpur/iso/pkg/manager/api/client"
	bisoUtils "github.com/bhojpur/iso/pkg/manager/api/client/utils"
	bisoContext "github.com/bhojpur/iso/pkg/manager/api/core/context"
	"github.com/bhojpur/iso/pkg/manager/api/core/types"
	"github.com/bhojpur/iso/pkg/manager/installer"
	"github.com/google/go-containerregistry/pkg/crane"
)

type opData struct {
	FinalRepo string
}

type resultData struct {
	Package bisoClient.Package
	Exists  bool
}

// The action can:
// 1: Build packages. Singularly (by specifying CURRENT_PACKAGE), or all of them.
//   (TODO: implement select  build only missing)
// 2: Download metadata for a given tree/repository
// 3: Create repository
var buildPackages = flag.Bool("build", false, "Build missing packages, or specified")
var download = flag.Bool("downloadMeta", false, "Download packages metadata")
var downloadAll = flag.Bool("downloadAllMeta", false, "Download All packages metadata")
var downloadFromList = flag.Bool("downloadFromList", false, "Download All packages metadata by listing all available image tags")

var fromIndex = flag.Bool("fromIndex", false, "Download metadata from index")
var buildx = flag.Bool("buildx", false, "Use docker buildx")

var createRepo = flag.Bool("createRepo", false, "create repository")
var onlyMissing = flag.Bool("onlyMissing", false, "Build only missing packages")
var push = flag.Bool("pushCache", false, "Pushing cache images while building")
var pushFinalImages = flag.Bool("pushFinalImages", false, "Pushing final images while building")
var pushFinalImagesRepository = flag.String("pushFinalImagesRepository", "", "Specify a different final repo")

var tree = flag.String("tree", "${PWD}/packages", "create repository")
var platform = flag.String("platform", "", "buildx platform")

var isomgrVersion = flag.String("isomgrVersion", "0.0.1", "default Bhojpur ISO version")
var isomgrArch = flag.String("isomgrArch", "amd64", "default Bhojpur ISO arch")
var values = flag.String("values", "", "Values file")

var outputdir = flag.String("output", "${PWD}/build", "output where to store packages")

var skipPackages = flag.String("skipPackages", "", "A space separated list of packages to skip")

//goaction:description Final container registry repository
var finalRepo = os.Getenv("FINAL_REPO")

//goaction:description Current package to build
var currentPackage = os.Getenv("CURRENT_PACKAGE")

//goaction:description Repository Name
var repositoryName = os.Getenv("REPOSITORY_NAME")

//goaction:description Repository Type
var repositoryType = os.Getenv("REPOSITORY_TYPE")

//goaction:description Optional pull cache repository
var pullRepository = os.Getenv("PULL_REPOSITORY")

//goaction:description Docker username to log into
var dockerUsername = os.Getenv("DOCKER_USERNAME")

//goaction:description Docker password to log into
var dockerPassword = os.Getenv("DOCKER_PASSWORD")

//goaction:description Optional docker endpoint, e.g. quay.io
var dockerEndpoint = os.Getenv("DOCKER_ENDPOINT")

func main() {
	flag.Parse()

	finalRepo = strings.ToLower(finalRepo)
	//	bisoUtils.RunSH("dependencies", "apk add curl")
	//	bisoUtils.RunSH("dependencies", "apk add docker")
	//	bisoUtils.RunSH("dependencies", "apk add jq")
	bisoUtils.RunSH("dependencies", "curl -L https://github.com/bhojpur/iso/releases/download/"+*isomgrVersion+"/isomgr-"+*isomgrVersion+"-linux-"+*isomgrArch+" --output isomgr")
	bisoUtils.RunSH("dependencies", "chmod +x isomgr")

	if *buildx {
		bisoUtils.RunSH("dependencies", "curl -L https://github.com/docker/buildx/releases/download/v0.7.1/buildx-v0.7.1.linux-amd64 --output docker-buildx")
		bisoUtils.RunSH("dependencies", "chmod a+x docker-buildx")
		bisoUtils.RunSH("dependencies", "mkdir -p ~/.docker/cli-plugins")
		bisoUtils.RunSH("dependencies", "mv docker-buildx ~/.docker/cli-plugins")
		bisoUtils.RunSH("dependencies", "docker buildx install")
		bisoUtils.RunSH("dependencies", "docker run --privileged --rm tonistiigi/binfmt --install all")
	}

	bisoUtils.RunSH("dependencies", "mv isomgr /usr/bin/isomgr && mkdir -p /etc/bhojpur/repos.conf.d/")
	bisoUtils.RunSH("dependencies", "curl -L https://raw.githubusercontent.com/bhojpur/repository-index/master/packages/iso.yml --output /etc/bhojpur/repos.conf.d/iso.yml")
	bisoUtils.RunSH("dependencies", "isomgr install -y system/isomgr")

	if dockerUsername != "" && dockerPassword != "" {
		out, err := bisoUtils.RunSHOUT("login", fmt.Sprintf(
			"echo %s | docker login -u %s --password-stdin %s",
			dockerPassword, dockerUsername, dockerEndpoint),
		)
		if err != nil {
			fmt.Println(string(out))
			os.Exit(1)
		}
	}

	switch {
	case *buildPackages:
		build()
	case *createRepo:
		create()
	case *download:
		downloadMeta()
	}
}

func repositoryPackages(repo string) (searchResult client.SearchResult) {

	fmt.Println("Retrieving remote repository packages")
	tmpdir, err := ioutil.TempDir(os.TempDir(), "ci")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmpdir)

	d := installer.NewSystemRepository(types.BhojpurRepository{
		Name:   repositoryName,
		Type:   repositoryType,
		Cached: true,
		Urls:   []string{repo},
	})

	ctx := bisoContext.NewContext()
	ctx.Config.System.Rootfs = "/"
	ctx.Config.System.TmpDirBase = tmpdir
	re, err := d.Sync(ctx, false)
	if err != nil {
		panic(err)
	} else {
		for _, p := range re.GetTree().GetDatabase().World() {
			searchResult.Packages = append(searchResult.Packages, client.Package{
				Name:     p.GetName(),
				Category: p.GetCategory(),
				Version:  p.GetVersion(),
			})
		}

		return
	}
}

func metaWorker(i int, wg *sync.WaitGroup, c <-chan bisoClient.Package, o opData) error {
	defer wg.Done()

	for p := range c {
		tmpdir, err := ioutil.TempDir(os.TempDir(), "ci")
		checkErr(err)
		unpackdir, err := ioutil.TempDir(os.TempDir(), "ci")
		checkErr(err)
		bisoUtils.RunSH("unpack", fmt.Sprintf("TMPDIR=%s XDG_RUNTIME_DIR=%s isomgr util unpack %s %s", tmpdir, tmpdir, p.ImageMetadata(o.FinalRepo), unpackdir))
		bisoUtils.RunSH("move", fmt.Sprintf("mv %s/* %s/", unpackdir, *outputdir))
		checkErr(err)
		os.RemoveAll(tmpdir)
		os.RemoveAll(unpackdir)
	}
	return nil
}

func buildWorker(i int, wg *sync.WaitGroup, c <-chan bisoClient.Package, o opData, results chan<- resultData) error {
	defer wg.Done()

	for p := range c {
		fmt.Println("Checking", p)
		results <- resultData{Package: p, Exists: p.ImageAvailable(o.FinalRepo)}
	}
	return nil
}

func create() {
	if *push {
		bisoUtils.RunSH(
			"create_repo",
			fmt.Sprintf(
				"isomgr create-repo --name '%s' --packages %s --tree %s --push-images --type docker 	--output %s",
				repositoryName, *outputdir, *tree, finalRepo,
			),
		)
	} else {
		bisoUtils.RunSH(
			"create_repo",
			fmt.Sprintf(
				"isomgr create-repo --name '%s' --packages %s --tree %s --type http --output %s",
				repositoryName, *outputdir, *tree, *outputdir,
			),
		)
	}
}

func build() {
	packs, err := bisoClient.TreePackages(*tree)
	checkErr(err)

	if *fromIndex {
		currentPackages := repositoryPackages(finalRepo)
		missingPackages := []client.Package{}
		skipP := []client.Package{}

		for _, f := range strings.Fields(*skipPackages) {
			pack, err := cmdhelpers.ParsePackageStr(f)
			if err == nil {
				skipP = append(skipP, client.Package{Name: pack.Name, Category: pack.Category})
			}
		}

		for _, p := range packs.Packages {
			if !client.Packages(currentPackages.Packages).Exist(p) ||
				len(skipP) != 0 && !client.Packages(skipP).Exist(client.Package{Name: p.Name, Category: p.Category}) {
				missingPackages = append(missingPackages, p)
			}
		}

		fmt.Println("Missing packages: " + fmt.Sprint(len(missingPackages)))
		for _, m := range missingPackages {
			fmt.Println("-", m.String())
		}

		for _, p := range missingPackages {
			buildPackage(p.String())
		}

		return
	}

	for _, p := range packs.Packages {
		if (*onlyMissing && !p.ImageAvailable(finalRepo) || !*onlyMissing) &&
			(currentPackage != "" && p.EqualSV(currentPackage) || currentPackage == "") {
			buildPackage(p.String())
		}
	}

	bisoUtils.RunSH("build perms", "chmod -R 777 "+*outputdir)
}

func buildPackage(s string) {
	fmt.Println("Building", s)

	args := []string{
		"isomgr",
		"build",
		"--only-target-package",
		"--pull",
		"--from-repositories",
		"--live-output",
	}

	if pullRepository != "" {
		args = append(args, "--pull-repository", pullRepository)
	}

	if *push {
		args = append(args, "--push")
	}

	if *buildx {
		args = append(args, "--backend-args", "--load")
	}

	if *platform != "" {
		args = append(args, "--backend-args", "--platform")
		args = append(args, "--backend-args", *platform)
	}

	if *values != "" {
		args = append(args, "--values", *values)
	}

	if *pushFinalImages {
		args = append(args, "--push-final-images")
	}

	if *pushFinalImagesRepository != "" {
		args = append(args, "--push-final-images-repository", *pushFinalImagesRepository)
	}

	if finalRepo != "" {
		args = append(args, "--image-repository", finalRepo)
	}
	if pullRepository != "" {
		args = append(args, "--pull-repository", pullRepository)
	}
	if *tree != "" {
		args = append(args, "--tree", *tree)
	}
	args = append(args, s)

	checkErr(bisoUtils.RunSH("build", strings.Join(args, " ")))
}

var defaultRetries int = 3

func retryList(image string, t int) ([]string, error) {
	tags, err := crane.ListTags(image)
	if err != nil {
		if t <= 0 {
			return tags, err
		}
		fmt.Printf("failed listing tags for '%s', retrying..\n", image)
		time.Sleep(time.Duration(defaultRetries-t+1) * time.Second)
		return retryList(image, t-1)
	}

	return tags, nil
}

func imageTags(tag string) ([]string, error) {
	return retryList(tag, defaultRetries)
}
func retryDownload(img, dest string, t int) error {
	if err := downloadImg(img, dest); err != nil {
		if t <= 0 {
			return err
		}
		fmt.Printf("failed downloading '%s', retrying..\n", img)
		time.Sleep(time.Duration(defaultRetries-t+1) * time.Second)
		return retryDownload(img, dest, t-1)
	}
	return nil
}

func downloadImg(img, dst string) error {
	tmpdir, err := ioutil.TempDir(os.TempDir(), "ci")
	if err != nil {
		return err
	}
	unpackdir, err := ioutil.TempDir(os.TempDir(), "ci")
	if err != nil {
		return err
	}
	err = bisoUtils.RunSH("unpack", fmt.Sprintf("TMPDIR=%s XDG_RUNTIME_DIR=%s isomgr util unpack %s %s", tmpdir, tmpdir, img, unpackdir))
	if err != nil {
		return err
	}
	err = bisoUtils.RunSH("move", fmt.Sprintf("mv %s/* %s/", unpackdir, dst))
	if err != nil {
		return err
	}
	os.RemoveAll(tmpdir)
	os.RemoveAll(unpackdir)
	return nil
}

func downloadImage(img, dst string) error {
	return retryDownload(img, dst, defaultRetries)
}

func downloadMeta() {

	var packs bisoClient.SearchResult

	if *downloadAll {
		var err error
		packs, err = bisoClient.TreePackages(*tree)
		checkErr(err)

		if *fromIndex {
			packs = repositoryPackages(finalRepo)
		}

		if *downloadFromList {
			tags, err := imageTags(finalRepo)
			checkErr(err)
			for _, t := range tags {
				if strings.HasSuffix(t, ".metadata.yaml") {
					img := fmt.Sprintf("%s:%s", finalRepo, t)
					fmt.Println("Downloading", img)
					checkErr(downloadImage(img, *outputdir))
				}
			}
			return
		}
	} else {
		var err error
		rpacks, err := bisoClient.TreePackages(*tree)
		checkErr(err)
		missingPackages := bisoClient.SearchResult{}

		currentPackages := repositoryPackages(finalRepo)
		skipP := []client.Package{}

		for _, f := range strings.Fields(*skipPackages) {
			pack, err := cmdhelpers.ParsePackageStr(f)
			if err == nil {
				skipP = append(skipP, client.Package{Name: pack.Name, Category: pack.Category})
			}
		}

		for _, p := range rpacks.Packages {
			if !client.Packages(currentPackages.Packages).Exist(p) ||
				len(skipP) != 0 && !client.Packages(skipP).Exist(client.Package{Name: p.Name, Category: p.Category}) {
				missingPackages.Packages = append(missingPackages.Packages, p)
			}
		}

		packs = missingPackages
	}

	all := make(chan bisoClient.Package)
	wg := new(sync.WaitGroup)

	for i := 0; i < 1; i++ {
		wg.Add(1)
		go metaWorker(i, wg, all, opData{FinalRepo: finalRepo})
	}

	for _, p := range packs.Packages {
		all <- p
	}
	close(all)
	wg.Wait()
}

func checkErr(err error) {
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
