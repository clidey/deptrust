package hook

import "testing"

func TestParseInstallCommand(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		want []packageRequest
	}{
		{
			name: "pnpm scoped package",
			cmd:  "pnpm add @clidey/ux@1.2.3",
			want: []packageRequest{{Ecosystem: "npm", Name: "@clidey/ux", Version: "1.2.3"}},
		},
		{
			name: "npm latest",
			cmd:  "npm install lodash",
			want: []packageRequest{{Ecosystem: "npm", Name: "lodash", Version: "latest"}},
		},
		{
			name: "pip exact",
			cmd:  "pip install requests==2.32.3",
			want: []packageRequest{{Ecosystem: "pypi", Name: "requests", Version: "2.32.3"}},
		},
		{
			name: "go get",
			cmd:  "go get golang.org/x/crypto@v0.31.0",
			want: []packageRequest{{Ecosystem: "go", Name: "golang.org/x/crypto", Version: "v0.31.0"}},
		},
		{
			name: "dotnet package",
			cmd:  "dotnet add package Newtonsoft.Json --version 13.0.3",
			want: []packageRequest{{Ecosystem: "nuget", Name: "Newtonsoft.Json", Version: "13.0.3"}},
		},
		{
			name: "maven artifact",
			cmd:  "mvn dependency:get -Dartifact=com.google.guava:guava:33.0.0",
			want: []packageRequest{{Ecosystem: "maven", Name: "com.google.guava:guava", Version: "33.0.0"}},
		},
		{
			name: "ignore test command",
			cmd:  "npm test",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseInstallCommand(tt.cmd)
			if len(got) != len(tt.want) {
				t.Fatalf("ParseInstallCommand() = %#v, want %#v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("ParseInstallCommand()[%d] = %#v, want %#v", i, got[i], tt.want[i])
				}
			}
		})
	}
}
