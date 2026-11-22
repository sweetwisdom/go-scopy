package tools

import "testing"

func TestParseAppDetailsInstallationFacts(t *testing.T) {
	apps := map[string]AppInfo{"com.example": {Package: "com.example", APKPath: "/data/app/base.apk"}}
	parseAppDetails("  Package [com.other] (1):\n    versionName=ignore\n  Package [com.example] (2):\n    versionCode=42 minSdk=21\n    versionName=2.0\n    firstInstallTime=2026-11-22 09:30:00\n", apps)
	app := apps["com.example"]
	if app.VersionName != "2.0" || app.VersionCode != "42" || app.InstalledAt.Format("2006-01-02 15:04:05") != "2026-11-22 09:30:00" || app.APKPath != "/data/app/base.apk" || len(apps) != 1 {
		t.Fatalf("incorrect installation facts: %+v", apps)
	}
	parseAppDetails("  Package [com.example] (2):\n    firstInstallTime=unknown\n", apps)
	if !apps["com.example"].InstalledAt.Equal(app.InstalledAt) {
		t.Fatal("invalid timestamp discarded known install time")
	}
}
