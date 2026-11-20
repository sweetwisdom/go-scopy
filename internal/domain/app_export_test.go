package domain

import (
	"strings"
	"testing"
)

func TestAppExportNameIsPortable(t *testing.T) {
	for _, test := range []struct{ pkg, version, want string }{
		{"com.demo", "1.0", "com.demo-1.0.apk"},
		{"com.demo", "", "com.demo.apk"},
		{"com.demo", "../beta:1", "com.demo-_beta_1.apk"},
	} {
		if got := AppExportName(test.pkg, test.version); got != test.want {
			t.Errorf("AppExportName(%q,%q)=%q, want %q", test.pkg, test.version, got, test.want)
		}
	}
	if len([]rune(AppExportName("com.demo", strings.Repeat("界", 1000)))) > 184 {
		t.Fatal("filename too long")
	}
}

func TestAppExportFilenamePrefersApplicationName(t *testing.T) {
	for _, test := range []struct {
		info AppInfo
		meta AppMeta
		want string
	}{
		{AppInfo{Package: "com.demo", Label: "演示应用", VersionName: "1.2.3"}, AppMeta{}, "演示应用-1.2.3.apk"},
		{AppInfo{Package: "com.demo", Label: "旧名称", VersionName: "1.2.3"}, AppMeta{Label: "当前名称"}, "当前名称-1.2.3.apk"},
		{AppInfo{Package: "com.demo", VersionCode: "123"}, AppMeta{}, "com.demo-123.apk"},
		{AppInfo{Package: "com.demo", Label: "Chrome"}, AppMeta{}, "Chrome.apk"},
		{AppInfo{Package: "com.demo", Label: "测试/应用", VersionName: "1:2"}, AppMeta{}, "测试_应用-1_2.apk"},
	} {
		if got := AppExportFilename(test.info, test.meta); got != test.want {
			t.Errorf("filename %q, want %q", got, test.want)
		}
	}
}
