package correction

import "testing"

func TestAcceptable(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		original  string
		corrected string
		expected  bool
	}{
		{
			name:     "an empty original has nothing to measure against",
			original: "", corrected: "anything at all", expected: false,
		},
		{
			name:     "an unchanged text",
			original: "Hej", corrected: "Hej", expected: true,
		},
		{
			name:     "a split compound rejoined moves no letters",
			original: "bo städer", corrected: "bostäder", expected: true,
		},
		{
			name:     "a missing letter restored",
			original: "Han gick til affären.", corrected: "Han gick till affären.", expected: true,
		},
		// The case the letter ratio exists for, kept as it was met: the model did
		// not join the split compound, it answered with likelier words. Measured
		// over the text as written this is inside MaximumChangeRatio, which is
		// why the second measure is not redundant.
		{
			name:     "likelier words in place of the same words spelled right",
			original: "Molntjänster och bo städer", corrected: "Miljarder till bostäder", expected: false,
		},
		{
			name:     "a substituted word of the same length",
			original: "Jag har en katt.", corrected: "Jag har en hund.", expected: false,
		},
		{
			name:     "a wholesale rewrite",
			original: "Jag gick till affären igår.", corrected: "I went to the store yesterday.", expected: false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if acceptable := Acceptable(testCase.original, testCase.corrected); acceptable != testCase.expected {
				t.Errorf("expected %t, got %t", testCase.expected, acceptable)
			}
		})
	}
}

// MinimumLetterChange is what keeps a short text from being immune to
// correction. Below thirty letters the ratio alone allows fewer than three
// edits, and below ten it allows none at all, so without the floor a short field
// could only ever be left as written. The case here is accepted solely because
// of it: two letters over fourteen is inside the floor and outside the ratio.
func TestAcceptableFloorAdmitsShortTexts(t *testing.T) {
	t.Parallel()

	const original = "Vi träffas imorn"

	if allowanceFromRatioAlone := int(float64(len(Letters(original))) * MaximumLetterChangeRatio); allowanceFromRatioAlone >= MinimumLetterChange {
		t.Fatalf(
			"this case no longer tests the floor: the ratio alone allows %d",
			allowanceFromRatioAlone,
		)
	}

	if !Acceptable(original, "Vi träffas i morgon") {
		t.Error("expected a short text's spelling fix to be admitted by the floor")
	}
}

// A known consequence rather than an intention. The measure over the text as
// written counts a capitalisation as an edit per character, so re-casing a short
// token exceeds MaximumChangeRatio on its own — "nasa" to "NASA" is four edits
// over four runes. Inside a sentence the same fix is a fraction of the text and
// passes. Pinned here so that tuning the ratios for typed text, where whole
// fields can be this short, starts from a stated behaviour rather than a
// surprise.
func TestAcceptableRejectsRecasingAShortToken(t *testing.T) {
	t.Parallel()

	if Acceptable("nasa", "NASA") {
		t.Error("expected re-casing a short token to be refused by the written-form ratio")
	}

	if !Acceptable("we sent it to nasa last week", "we sent it to NASA last week") {
		t.Error("expected the same fix inside a sentence to be accepted")
	}
}

func TestLetters(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		text     string
		expected string
	}{
		{name: "empty", text: "", expected: ""},
		{name: "spacing and punctuation are dropped", text: "Hej, du!", expected: "hejdu"},
		{name: "case is dropped", text: "NASA", expected: "nasa"},
		{name: "digits are carried", text: "rum 12b", expected: "rum12b"},
		{name: "non-ascii letters are carried", text: "Vår Höst", expected: "vårhöst"},
		{name: "only punctuation", text: "—!?", expected: ""},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if letters := string(Letters(testCase.text)); letters != testCase.expected {
				t.Errorf("expected %q, got %q", testCase.expected, letters)
			}
		})
	}
}

// What Letters is for: the texts a split compound differs by compare as equal
// once it is applied, which is what lets the letter measure be as tight as it is.
func TestLettersIgnoresHowWordsAreSplit(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		a    string
		b    string
	}{
		{name: "a split compound", a: "bo städer", b: "bostäder"},
		{name: "a hyphenated compound", a: "e-post", b: "epost"},
		{name: "differing punctuation", a: "Hej då!", b: "hejdå"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if string(Letters(testCase.a)) != string(Letters(testCase.b)) {
				t.Errorf("expected %q and %q to reduce alike, got %q and %q",
					testCase.a, testCase.b, string(Letters(testCase.a)), string(Letters(testCase.b)))
			}
		})
	}
}
