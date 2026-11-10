package app

import (
	"context"
	_ "embed"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/clidey/deptrust/internal/models"
)

//go:embed runtime/MavenSources.java
var mavenSourceInspector string

//go:embed runtime/gradle-sources.init.gradle
var gradleSourceInspector string

func jvmProjectRestriction(ctx context.Context, q models.Query) (string, error) {
	has := func(file string) bool { _, err := os.Stat(filepath.Join(q.ProjectDir, file)); return err == nil }
	maven := has("pom.xml")
	gradle := has("settings.gradle") || has("settings.gradle.kts") || has("build.gradle") || has("build.gradle.kts")
	if maven == gradle {
		return "", fmt.Errorf("select an unambiguous Maven or Gradle project directory")
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	dir, err := os.MkdirTemp("", "deptrust-jvm-sources-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	output := filepath.Join(dir, "sources.report")
	env := append(os.Environ(), "DEPTRUST_SOURCE_PACKAGE="+q.Package, "DEPTRUST_SOURCE_OUTPUT="+output)
	if maven {
		return inspectMavenSources(ctx, q, dir, output, env)
	}
	script := filepath.Join(dir, "sources.init.gradle")
	if err := os.WriteFile(script, []byte(gradleSourceInspector), 0600); err != nil {
		return "", err
	}
	taskName := "deptrustInspectSources" + strings.TrimPrefix(filepath.Base(dir), "deptrust-jvm-sources-")
	env = append(env, "DEPTRUST_SOURCE_PROJECT="+q.ProjectDir, "DEPTRUST_SOURCE_TASK="+taskName)
	cmd := exec.CommandContext(ctx, "gradle", "--offline", "--no-daemon", "--no-configuration-cache", "--console=plain", "--init-script", script, taskName)
	cmd.Dir = q.ProjectDir
	cmd.Env = env
	// Gradle runs only the generated inspection task. A complete report proves
	// that project-local resolution ran in the task lifecycle.
	_ = cmd.Run()
	data, err := os.ReadFile(output)
	if err != nil {
		return "", fmt.Errorf("cannot inspect effective Gradle sources offline; Gradle and cached project configuration are required")
	}
	var report struct {
		Complete     bool     `json:"complete"`
		Declared     bool     `json:"declared"`
		Unsupported  bool     `json:"unsupported"`
		Repositories []string `json:"repositories"`
		Resolved     bool     `json:"resolved"`
	}
	if err := json.Unmarshal(data, &report); err != nil || !report.Complete {
		return "", fmt.Errorf("invalid Gradle source report")
	}
	return classifyJVMRepositories(report.Declared && !report.Unsupported && report.Resolved, report.Repositories)
}

func inspectMavenSources(ctx context.Context, q models.Query, dir, output string, env []string) (string, error) {
	version := exec.CommandContext(ctx, "mvn", "-Dstyle.color=never", "--version")
	version.Env = env
	info, err := version.Output()
	if err != nil {
		return "", fmt.Errorf("installed Maven is required to inspect project sources")
	}
	mavenHome := ""
	for _, line := range strings.Split(string(info), "\n") {
		if strings.HasPrefix(line, "Maven home:") {
			mavenHome = strings.TrimSpace(strings.TrimPrefix(line, "Maven home:"))
		}
	}
	if mavenHome == "" || !filepath.IsAbs(mavenHome) {
		return "", fmt.Errorf("cannot locate Maven SDK libraries")
	}
	source := filepath.Join(dir, "MavenSources.java")
	if err := os.WriteFile(source, []byte(mavenSourceInspector), 0600); err != nil {
		return "", err
	}
	classes := filepath.Join(dir, "classes")
	if err := os.Mkdir(classes, 0700); err != nil {
		return "", err
	}
	compile := exec.CommandContext(ctx, "javac", "-cp", filepath.Join(mavenHome, "lib", "*"), "-d", classes, source)
	compile.Env = env
	if err := compile.Run(); err != nil {
		return "", fmt.Errorf("cannot compile Maven source inspector; a compatible JDK is required")
	}
	metadata := filepath.Join(classes, "META-INF", "plexus")
	if err := os.MkdirAll(metadata, 0700); err != nil {
		return "", err
	}
	descriptor := `<component-set><components><component><role>org.apache.maven.AbstractMavenLifecycleParticipant</role><role-hint>deptrust-source-inspector</role-hint><implementation>MavenSources</implementation></component></components></component-set>`
	if err := os.WriteFile(filepath.Join(metadata, "components.xml"), []byte(descriptor), 0600); err != nil {
		return "", err
	}
	extension := filepath.Join(dir, "inspector.jar")
	pack := exec.CommandContext(ctx, "jar", "--create", "--file", extension, "-C", classes, ".")
	pack.Env = env
	if err := pack.Run(); err != nil {
		return "", fmt.Errorf("cannot package Maven source inspector")
	}
	cmd := exec.CommandContext(ctx, "mvn", "--offline", "--batch-mode", "-Dstyle.color=never", "-Dmaven.ext.class.path="+extension, "validate")
	cmd.Dir = q.ProjectDir
	cmd.Env = env
	_ = cmd.Run()
	data, err := os.ReadFile(output)
	if err != nil {
		return "", fmt.Errorf("cannot inspect effective Maven sources offline; parent POMs/extensions must already be cached")
	}
	var report struct {
		Entries []struct {
			Key   string `xml:"key,attr"`
			Value string `xml:",chardata"`
		} `xml:"entry"`
	}
	if err := xml.Unmarshal(data, &report); err != nil {
		return "", fmt.Errorf("invalid Maven source report")
	}
	complete, declared := false, false
	var repositories []string
	for _, entry := range report.Entries {
		switch entry.Key {
		case "complete":
			complete = entry.Value == "true"
		case "declared":
			declared = entry.Value == "true"
		default:
			if strings.HasPrefix(entry.Key, "source.") {
				repositories = append(repositories, entry.Value)
			}
		}
	}
	if !complete {
		return "", fmt.Errorf("incomplete Maven source report")
	}
	return classifyJVMRepositories(declared, repositories)
}

func classifyJVMRepositories(declared bool, repositories []string) (string, error) {
	const reason = "Effective JVM project does not exclusively establish this declared module from Maven Central; public requests were skipped. Assessment is unavailable."
	if !declared || len(repositories) == 0 {
		return reason, nil
	}
	for _, repository := range repositories {
		if !publicSource(repository, "repo.maven.apache.org", "/maven2") && !publicSource(repository, "repo1.maven.org", "/maven2") {
			return reason, nil
		}
	}
	return "", nil
}
