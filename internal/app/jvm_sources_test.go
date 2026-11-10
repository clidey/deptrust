package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/clidey/deptrust/internal/models"
)

func TestEffectiveMavenProjectSources(t *testing.T) {
	if os.Getenv("DEPTRUST_TEST_JVM") != "1" {
		t.Skip("set DEPTRUST_TEST_JVM=1 for installed JVM integration")
	}
	for _, tool := range []string{"mvn", "javac", "jar"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Fatal(tool + " required")
		}
	}
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); w.WriteHeader(502) }))
	defer server.Close()
	for _, tt := range []struct {
		name, repositories, settings string
		blocked                      bool
	}{
		{"public", "", "", false},
		{"profile repository", "", `<profiles><profile><id>internal</id><repositories><repository><id>internal</id><url>` + server.URL + `</url></repository></repositories></profile></profiles><activeProfiles><activeProfile>internal</activeProfile></activeProfiles>`, true},
		{"mirrored public", "", `<mirrors><mirror><id>internal</id><mirrorOf>*</mirrorOf><url>` + server.URL + `</url></mirror></mirrors>`, true},
		{"project repository", `<repositories><repository><id>private</id><url>` + server.URL + `</url></repository></repositories>`, "", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			settings := filepath.Join(dir, "settings.xml")
			if err := os.WriteFile(settings, []byte(`<settings>`+tt.settings+`</settings>`), 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("MAVEN_ARGS", "-s "+settings)
			t.Setenv("MAVEN_SKIP_RC", "1")
			pom := `<project><modelVersion>4.0.0</modelVersion><groupId>fixture</groupId><artifactId>source-test</artifactId><version>1.0</version><dependencies><dependency><groupId>company</groupId><artifactId>secret</artifactId><version>1.0</version></dependency></dependencies>` + tt.repositories + `</project>`
			if err := os.WriteFile(filepath.Join(dir, "pom.xml"), []byte(pom), 0600); err != nil {
				t.Fatal(err)
			}
			q := models.Query{Ecosystem: models.EcosystemMaven, Package: "company:secret", ProjectDir: dir}
			reason, err := sourceRestriction(context.Background(), q)
			if err != nil || (reason != "") != tt.blocked {
				t.Fatalf("%q %v", reason, err)
			}
			if calls.Load() != 0 {
				t.Fatal("Maven contacted a dependency repository")
			}
			if _, err := os.Stat(filepath.Join(dir, "target")); !os.IsNotExist(err) {
				t.Fatal("Maven build ran")
			}
		})
	}
}

func TestEffectiveGradleProjectSources(t *testing.T) {
	if os.Getenv("DEPTRUST_TEST_JVM") != "1" {
		t.Skip("set DEPTRUST_TEST_JVM=1 for installed JVM integration")
	}
	if _, err := exec.LookPath("gradle"); err != nil {
		t.Fatal("Gradle required")
	}
	t.Setenv("GRADLE_USER_HOME", t.TempDir())
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); w.WriteHeader(502) }))
	defer server.Close()
	for _, tt := range []struct {
		name, repositories, settings, extra, pkg, dependency string
		blocked, warm                                        bool
	}{
		{name: "public cached module", repositories: "repositories { mavenCentral() }", pkg: "org.apache.commons:commons-lang3", dependency: "org.apache.commons:commons-lang3:3.20.0", warm: true},
		{name: "Maven mirror reuse", repositories: "repositories { mavenCentral() }", pkg: "org.apache.commons:commons-lang3", dependency: "org.apache.commons:commons-lang3:3.20.0", warm: true, blocked: true},
		{name: "settings private", settings: `dependencyResolutionManagement { repositories { maven { url = uri('` + server.URL + `'); allowInsecureProtocol = true } } }`, pkg: "company:secret", dependency: "company:secret:1.0", blocked: true},
		{name: "exclusive private", repositories: `repositories { mavenCentral(); exclusiveContent { forRepository { maven { url = uri('` + server.URL + `'); allowInsecureProtocol = true } }; filter { includeGroup('company') } } }`, pkg: "company:secret", dependency: "company:secret:1.0", blocked: true},
		{name: "project substitution", repositories: "repositories { mavenCentral() }", settings: "include ':local'\n", extra: "configurations.inspection.resolutionStrategy.dependencySubstitution { substitute module('company:secret') using project(':local') }\n", pkg: "company:secret", dependency: "company:secret:1.0", blocked: true},
		{name: "missing offline metadata", repositories: "repositories { mavenCentral() }", pkg: "company:secret", dependency: "company:secret:1.0", blocked: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "settings.gradle"), []byte("rootProject.name='source-test'\n"+tt.settings), 0600); err != nil {
				t.Fatal(err)
			}
			if tt.name == "project substitution" {
				if err := os.Mkdir(filepath.Join(dir, "local"), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "local/build.gradle"), []byte("plugins { id 'java-library' }\ngroup='company'\nversion='1.0'\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			build := tt.repositories + "\nconfigurations { inspection }\ndependencies { inspection '" + tt.dependency + "' }\n" + tt.extra + "\ntasks.named('help') { doLast { new File(projectDir, 'task-ran').text='bad' } }\ntasks.register('deptrustPrime') { doLast { configurations.inspection.incoming.resolutionResult.allComponents.size() } }\n"
			if err := os.WriteFile(filepath.Join(dir, "build.gradle"), []byte(build), 0600); err != nil {
				t.Fatal(err)
			}
			if tt.warm {
				// Explicit fixture setup fetches only a previously vetted public module.
				// The product inspection that follows is offline and must not run help.
				prime := exec.Command("gradle", "--no-daemon", "--console=plain", "deptrustPrime")
				prime.Dir = dir
				if out, err := prime.CombinedOutput(); err != nil {
					t.Fatalf("public fixture cache setup: %s %v", out, err)
				}
			}
			if tt.name == "Maven mirror reuse" {
				home := t.TempDir()
				if err := os.Mkdir(filepath.Join(home, ".m2"), 0700); err != nil {
					t.Fatal(err)
				}
				mirrors := `<settings><mirrors><mirror><id>internal</id><mirrorOf>*</mirrorOf><url>` + server.URL + `</url></mirror></mirrors></settings>`
				if err := os.WriteFile(filepath.Join(home, ".m2/settings.xml"), []byte(mirrors), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "gradle.properties"), []byte("org.gradle.mirror.maven.settings=true\n"), 0600); err != nil {
					t.Fatal(err)
				}
				t.Setenv("JAVA_TOOL_OPTIONS", "-Duser.home="+home)
			}
			q := models.Query{Ecosystem: models.EcosystemMaven, Package: tt.pkg, ProjectDir: dir}
			reason, err := sourceRestriction(context.Background(), q)
			if err != nil || (reason != "") != tt.blocked {
				t.Fatalf("%q %v", reason, err)
			}
			if calls.Load() != 0 {
				t.Fatal("Gradle contacted a private dependency repository")
			}
			if _, err := os.Stat(filepath.Join(dir, "task-ran")); !os.IsNotExist(err) {
				t.Fatal("Gradle task executed")
			}
		})
	}
}

func TestMavenLocalDependencyIdentities(t *testing.T) {
	if os.Getenv("DEPTRUST_TEST_JVM") != "1" {
		t.Skip("set DEPTRUST_TEST_JVM=1")
	}
	for _, kind := range []string{"system", "reactor", "inherited private"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			settings := filepath.Join(dir, "settings.xml")
			if err := os.WriteFile(settings, []byte("<settings/>"), 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("MAVEN_ARGS", "-s "+settings)
			t.Setenv("MAVEN_SKIP_RC", "1")
			dependency := `<dependency><groupId>company</groupId><artifactId>secret</artifactId><version>1.0</version>`
			extra := ""
			if kind == "system" {
				dependency += `<scope>system</scope><systemPath>${project.basedir}/fake.jar</systemPath>`
				if err := os.WriteFile(filepath.Join(dir, "fake.jar"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			dependency += `</dependency>`
			if kind == "reactor" {
				extra = `<packaging>pom</packaging><modules><module>child</module></modules>`
				if err := os.Mkdir(filepath.Join(dir, "child"), 0700); err != nil {
					t.Fatal(err)
				}
				child := `<project><modelVersion>4.0.0</modelVersion><groupId>company</groupId><artifactId>secret</artifactId><version>1.0</version></project>`
				if err := os.WriteFile(filepath.Join(dir, "child/pom.xml"), []byte(child), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "inherited private" {
				extra = `<parent><groupId>fixture</groupId><artifactId>parent</artifactId><version>1.0</version><relativePath>parent.xml</relativePath></parent>`
				parent := `<project><modelVersion>4.0.0</modelVersion><groupId>fixture</groupId><artifactId>parent</artifactId><version>1.0</version><packaging>pom</packaging><repositories><repository><id>private</id><url>https://private.invalid</url></repository></repositories></project>`
				if err := os.WriteFile(filepath.Join(dir, "parent.xml"), []byte(parent), 0600); err != nil {
					t.Fatal(err)
				}
			}
			pom := `<project><modelVersion>4.0.0</modelVersion>` + extra + `<groupId>fixture</groupId><artifactId>root</artifactId><version>1.0</version><dependencies>` + dependency + `</dependencies></project>`
			if err := os.WriteFile(filepath.Join(dir, "pom.xml"), []byte(pom), 0600); err != nil {
				t.Fatal(err)
			}
			reason, err := sourceRestriction(context.Background(), models.Query{Ecosystem: models.EcosystemMaven, Package: "company:secret", ProjectDir: dir})
			if err != nil || reason == "" {
				t.Fatalf("%q %v", reason, err)
			}
		})
	}
}

func TestJVMSourceClassification(t *testing.T) {
	for _, tt := range []struct {
		declared bool
		urls     []string
		blocked  bool
	}{
		{true, []string{"https://repo.maven.apache.org/maven2/"}, false},
		{true, []string{"https://repo.maven.apache.org/maven2/", "https://private.invalid"}, true},
		{true, []string{"file:///private/repository"}, true},
		{false, []string{"https://repo.maven.apache.org/maven2/"}, true},
		{true, nil, true},
	} {
		reason, err := classifyJVMRepositories(tt.declared, tt.urls)
		if err != nil || (reason != "") != tt.blocked {
			t.Fatalf("%q %v", reason, err)
		}
	}
	dir := t.TempDir()
	for _, file := range []string{"pom.xml", "build.gradle"} {
		if err := os.WriteFile(filepath.Join(dir, file), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := sourceRestriction(context.Background(), models.Query{Ecosystem: models.EcosystemMaven, Package: "company:secret", ProjectDir: dir}); err == nil {
		t.Fatal("ambiguous project manager must stop before execution")
	}
}
