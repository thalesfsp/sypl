package debug

import (
	"os"
	"testing"

	"github.com/thalesfsp/sypl/v2/level"
	"github.com/thalesfsp/sypl/v2/shared"
)

func TestNew(t *testing.T) {
	type args struct {
		componentName string
		outputName    string
	}
	tests := []struct {
		name        string
		args        args
		content     string
		wantLevel   level.Level
		wantMatcher Matcher
		wantOK      bool
	}{
		{
			name: "Should work - COL",
			args: args{
				componentName: fixtureComponentX,
				outputName:    fixtureOutputY,
			},
			content:     fixtureInfoComponentXOutputYDebugOutputZTrace,
			wantLevel:   level.Debug,
			wantMatcher: COL,
			wantOK:      true,
		},
		{
			name: "Should work - OL",
			args: args{
				componentName: fixtureComponentX,
				outputName:    "outputZ",
			},
			content:     fixtureInfoComponentXOutputYDebugOutputZTrace,
			wantLevel:   level.Trace,
			wantMatcher: OL,
			wantOK:      true,
		},
		{
			name: "Should work - L",
			args: args{
				componentName: fixtureComponentX,
				outputName:    "outputW",
			},
			content:     fixtureInfoComponentXOutputYDebugOutputZTrace,
			wantLevel:   level.Info,
			wantMatcher: L,
			wantOK:      true,
		},
		{
			name: "Should work - level none",
			args: args{
				componentName: fixtureComponentX,
				outputName:    fixtureOutputY,
			},
			content:     "info,componentX:outputY:none,outputZ:trace",
			wantLevel:   level.None,
			wantMatcher: COL,
			wantOK:      true,
		},
		{
			name: "Should fail - no match",
			args: args{
				componentName: fixtureComponentX,
				outputName:    "outputW",
			},
			content:     "componentX:outputY:debug,outputZ:trace,info",
			wantLevel:   level.None,
			wantMatcher: None,
			wantOK:      false,
		},
		{
			name: "Should fail - invalid level",
			args: args{
				componentName: fixtureComponentX,
				outputName:    fixtureOutputY,
			},
			content:     "componentX:outputY:asd,outputZ:trace,info",
			wantLevel:   level.None,
			wantMatcher: None,
			wantOK:      false,
		},
		{
			name: "Should do nothing",
			args: args{
				componentName: fixtureComponentX,
				outputName:    fixtureOutputY,
			},
			wantLevel:   level.None,
			wantMatcher: None,
			wantOK:      false,
		},
		{
			name:        "Should fail - component dot must not match another name",
			args:        args{componentName: "a.b", outputName: "literal-output"},
			content:     "aXb:literal-output:debug",
			wantLevel:   level.None,
			wantMatcher: None,
			wantOK:      false,
		},
		{
			name:        "Should fail - output dot must not match another name",
			args:        args{componentName: "literal-component", outputName: "a.b"},
			content:     "aXb:debug",
			wantLevel:   level.None,
			wantMatcher: None,
			wantOK:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(shared.LevelEnvVar, tt.content)
			defer os.Unsetenv(shared.LevelEnvVar)

			d := New(tt.args.componentName, tt.args.outputName)

			lvl, m, ok := d.Level()

			if ok != tt.wantOK {
				t.Fatalf("OK; expected %+v got %+v", tt.wantOK, ok)
			}

			if m != tt.wantMatcher {
				t.Fatalf("OK; expected %+v got %+v", tt.wantMatcher, m)
			}

			if lvl != tt.wantLevel {
				t.Fatalf("OK; expected %+v got %+v", tt.wantLevel, lvl)
			}
		})
	}

	// Names are literals, including unmatched delimiters and other regexp syntax.
	for _, name := range []string{"api(v2", "C++ service", "*x", `a\q`, "x{99999}", `()[]{}*+?|^$\.`} {
		t.Run("Should work - literal names "+name, func(t *testing.T) {
			t.Setenv(shared.LevelEnvVar, name+":"+name+":debug")

			lvl, matcher, ok := New(name, name).Level()
			if !ok || lvl != level.Debug || matcher != COL {
				t.Fatalf("Level() = (%v, %v, %v), want (debug, %v, true)", lvl, matcher, ok, COL)
			}
		})
	}
}
