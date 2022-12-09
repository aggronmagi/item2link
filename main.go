package main

import (
	"bytes"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/pflag"
	yaml "gopkg.in/yaml.v2"
)

var pcfg = struct {
	OutputPath      string
	TemplateExcepts []string
	TemplateProfile []string
	ExportExpect    bool
}{
	OutputPath: "~/Library/Application Support/iTerm2/DynamicProfiles",
}

func init() {
	pflag.StringVarP(&pcfg.OutputPath, "output", "o", pcfg.OutputPath, "生成文件输出目录")
	pflag.StringSliceVarP(&pcfg.TemplateExcepts, "expect", "e", pcfg.TemplateExcepts, "expect模板文件目录")
	pflag.StringSliceVarP(&pcfg.TemplateProfile, "profile", "p", pcfg.TemplateProfile, "profile模板文件目录")
}

type buildConfig struct {
	Basic    map[string]string   // 基础配置
	Services []map[string]string //覆盖配置
}

func main() {
	pflag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		pflag.PrintDefaults()
	}
	pflag.Parse()
	if pflag.NArg() < 1 {
		fmt.Println("not set config file or config file dir")
		return
	}

	if strings.HasPrefix(pcfg.OutputPath, "~") {
		pcfg.OutputPath = strings.Replace(pcfg.OutputPath, "~", os.Getenv("HOME"), 1)
	}
	for _, filename := range pflag.Args() {
		if IsDir(filename) {
			files, failed := GetAllFileWithExt(filename, ".yaml")
			if failed {
				log.Fatalln("读取目录", filename, "错误")
			}
			for _, v := range files {
				parse(v)
			}
		} else {
			parse(filename)
		}
	}
}

//go:embed profiles/*.json
var profileDir embed.FS

func parse(cfgFileName string) {
	config := &buildConfig{}
	data, err := ioutil.ReadFile(cfgFileName)
	if err != nil {
		log.Fatalln("Read ", cfgFileName, " Error.", err)
	}

	// 解析结构体
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		log.Fatalln("Unmarshal ", cfgFileName, " Error.", err)
	}
	profileName, pset := config.Basic["profile"]
	if !pset {
		profileName = "ssh.json"
	}
	if strings.HasPrefix(profileName, "~") {
		profileName = strings.Replace(profileName, "~", os.Getenv("HOME"), 1)
	}

	var profileData []byte

	for _, dir := range pcfg.TemplateProfile {
		if IsDir(dir) {
			profileData, err = ioutil.ReadFile(filepath.Join(dir, profileName))
			if err != nil {
				log.Println("warn: read ", profileName, " from ", dir, " failed.", err)
				continue
			}
			break
		}
	}
	if profileData == nil {
		profileData, err = profileDir.ReadFile(filepath.Join("profiles", profileName))
		if err != nil {
			log.Println("warn: read ", profileName, " from embed files failed.", err)
		}
	}
	if profileData == nil {
		log.Fatal("can't find profile", profileName)
	}

	buf := bytes.Buffer{}
	buf.WriteString(`{
	"Profiles": [`)

	for k, v := range config.Services {
		basic := make(map[string]string) // 基础配置
		for sk, sv := range config.Basic {
			basic[sk] = sv
		}
		for sk, sv := range v {
			basic[sk] = sv
		}
		if name, ok := basic["name"]; ok {
			basic["guid"] = base64.RawStdEncoding.EncodeToString([]byte(name))
		} else {
			log.Println("ignore case, not set name")
			log.Println("allConfig:", basic)
			continue
		}
		if _, ok := basic["badge_text"]; !ok {
			basic["badge_text"] = basic["name"]
		}
		if _, ok := basic["tab_text"]; !ok {
			basic["tab_text"] = basic["name"]
		}
		strConfig := string(profileData)
		for sk, sv := range basic {
			strConfig = strings.ReplaceAll(strConfig, "$"+sk, sv)
		}
		strConfig = strings.ReplaceAll(strConfig, "$index", fmt.Sprintf("%d", k+1))
		if k > 0 {
			buf.WriteString(`,
	`)
		}
		buf.WriteString(strConfig)
	}
	buf.WriteString(`	]
}`)

	var v interface{}
	var newData []byte
	err = json.Unmarshal(buf.Bytes(), v)
	if err == nil {
		newData, err = json.MarshalIndent(v, "", "\t")
		if err == nil {
			ioutil.WriteFile(pcfg.OutputPath+"/"+strings.Replace(filepath.Base(cfgFileName), ".yaml", ".json", 1),
				newData, 0644)
			return
		}
	}
	dst := &bytes.Buffer{}
	err = json.Indent(dst, buf.Bytes(), "", "\t")
	if err == nil {
		ioutil.WriteFile(pcfg.OutputPath+"/"+strings.Replace(filepath.Base(cfgFileName), ".yaml", ".json", 1),
			dst.Bytes(), 0644)
		return
	}

	log.Println("Error: ", err)
	ioutil.WriteFile(pcfg.OutputPath+"/"+strings.Replace(filepath.Base(cfgFileName), ".yaml", ".json", 1),
		buf.Bytes(), 0644)
}

// IsDir 判断所给路径是否为文件夹
func IsDir(path string) bool {
	s, err := os.Stat(path)
	if err != nil {
		return false
	}
	return s.IsDir()
}

// IsFile 判断所给路径是否为文件
func IsFile(path string) bool {
	return !IsDir(path)
}

// GetAllFileWithExt 获取某个目录下所有ext后缀的文件
func GetAllFileWithExt(path, ext string) (files []string, failed bool) {
	return GetAllFile(path, func(file string) bool {
		if ext == "*" {
			return false
		}
		return filepath.Ext(file) != ext
	})
}

// GetAllFile 获取某个目录下所有文件
func GetAllFile(path string, filter func(string) bool) (files []string, failed bool) {
	if !IsDir(path) {
		return
	}
	rd, err := ioutil.ReadDir(path)
	if err != nil {
		log.Println("Error: 打开目录 ", path, " 出错.")
		failed = true
		return
	}
	for _, fi := range rd {
		if fi.IsDir() {
			// fmt.Printf("[%s]\n", path+"/"+fi.Name())
			nfs, failure := GetAllFile(path+fi.Name()+"/", filter)
			if !failure && len(nfs) > 1 {
				files = append(files, nfs...)
			}
		} else {
			if filter(fi.Name()) {
				continue
			}
			files = append(files, path+fi.Name())
			// fmt.Println(fi.Name())
		}
	}
	return
}
