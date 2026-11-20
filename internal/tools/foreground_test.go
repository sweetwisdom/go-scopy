package tools

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestResumedAppsAreScopedToDisplays(t *testing.T) {
	text := `ACTIVITY MANAGER ACTIVITIES (dumpsys activity activities)
  mResumedActivity: ActivityRecord{global u0 com.wrong.global/.Activity t9}
  Display #2 (activities from top to bottom):
    topResumedActivity=ActivityRecord{222 u10 org.virtual.app/org.virtual.app.Main t20}
    mResumedActivity: ActivityRecord{background u0 com.other.visible/.Main t30}
  Display #0 (activities from top to bottom):
    mResumedActivity: ActivityRecord{abc u0 com.example.main/.Main$Inner t3}
  Display #4 (activities from top to bottom):
    topResumedActivity=null
  Global summary:
    mResumedActivity: ActivityRecord{global u0 com.wrong.summary/.Main t9}
`
	want := map[int]string{0: "com.example.main", 2: "org.virtual.app"}
	if got := resumedApps(text); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got := resumedApps("mCurrentFocus=Window{abc u0 com.wrong.focus/.Main}"); len(got) != 0 {
		t.Fatalf("accepted unscoped focus: %v", got)
	}
}

func TestForegroundAppRejectsLauncherAndUnknownResults(t *testing.T) {
	for _, tc := range []struct {
		name, activity, launcher, want string
		fail                           bool
	}{
		{"app", "Display #0:\n  mResumedActivity: ActivityRecord{abc u0 com.example.app/.Main}", "com.vendor.home/.Launcher", "com.example.app", false},
		{"launcher", "Display #0:\n  topResumedActivity=ActivityRecord{abc u0 com.vendor.home/.Launcher}", "com.vendor.home/.Launcher", "", false},
		{"no main", "Display #8:\n  mResumedActivity: ActivityRecord{abc u0 com.example.app/.Main}", "com.vendor.home/.Launcher", "", false},
		{"system", "Display #0:\n  mResumedActivity: ActivityRecord{abc u0 com.android.systemui/.Main}", "com.vendor.home/.Launcher", "", false},
		{"unknown launcher", "Display #0:\n  mResumedActivity: ActivityRecord{abc u0 com.example.app/.Main}", "No activity found", "", false},
		{"adb failed", "", "", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := foregroundApp(context.Background(), func(_ context.Context, args ...string) ([]byte, error) {
				if tc.fail {
					return nil, errors.New("offline")
				}
				if strings.Contains(strings.Join(args, " "), "dumpsys") {
					return []byte(tc.activity), nil
				}
				if !strings.Contains(strings.Join(args, " "), "--user current") {
					t.Fatal("launcher resolved for the wrong user")
				}
				return []byte(tc.launcher), nil
			})
			if got != tc.want || (err != nil) != (tc.want == "") {
				t.Fatalf("got %q, %v", got, err)
			}
		})
	}
}

func TestReturnHomeWaitsForVirtualAppAndTargetsMainDisplay(t *testing.T) {
	reads := 0
	var commands [][]string
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := returnHomeAfterApp(ctx, "com.example.app", map[int]bool{0: true, 2: true}, func(_ context.Context, args ...string) ([]byte, error) {
		commands = append(commands, args)
		if len(args) > 1 && args[1] == "dumpsys" {
			reads++
			text := "Display #0:\n  mResumedActivity: ActivityRecord{abc u0 com.example.app/.Main}\nDisplay #2:\n  topResumedActivity=ActivityRecord{old u0 com.example.app/.Main}\n"
			if reads > 1 {
				text += "Display #3:\n  topResumedActivity=ActivityRecord{def u0 com.example.app/.Main}"
			}
			return []byte(text), nil
		}
		if reads < 2 {
			t.Fatal("HOME sent before app appeared in virtual display")
		}
		return nil, nil
	})
	if err != nil || !reflect.DeepEqual(commands[len(commands)-1], []string{"shell", "input", "-d", "0", "keyevent", "3"}) {
		t.Fatalf("err=%v, commands=%v", err, commands)
	}
}

func TestReturnHomeDoesNotSendKeysOnFailure(t *testing.T) {
	for _, mode := range []string{"timeout", "offline", "cancelled", "existing display", "wrong app"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			if mode == "cancelled" {
				cancel()
			}
			err := returnHomeAfterApp(ctx, "com.example.app", map[int]bool{0: true, 2: true}, func(_ context.Context, args ...string) ([]byte, error) {
				if args[1] != "dumpsys" {
					t.Fatalf("sent keys on failure: %v", args)
				}
				if mode == "offline" {
					return nil, errors.New("offline")
				}
				pkg := "com.other.app"
				if mode == "existing display" {
					pkg = "com.example.app"
				}
				return []byte("Display #2:\n  mResumedActivity: ActivityRecord{abc u0 " + pkg + "/.Main}"), nil
			})
			if err == nil {
				t.Fatal("missing failure")
			}
		})
	}
}
