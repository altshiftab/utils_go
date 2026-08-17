package dmarc

import (
	"errors"
	"reflect"
	"testing"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
)

func TestParseDmarcRecord(t *testing.T) {
	t.Parallel()

	type args struct {
		data []byte
	}
	tests := []struct {
		name    string
		args    args
		want    *Record
		wantErr bool
		wantIs  error // expected root error via errors.Is
	}{
		{
			name: "minimal valid record",
			args: args{data: []byte("v=DMARC1; p=none")},
			want: &Record{
				Raw: "v=DMARC1; p=none",
				P:   "none",
			},
			wantErr: false,
			wantIs:  nil,
		},
		{
			name: "full valid record with spaces and trailing semicolon",
			args: args{data: []byte("v=DMARC1 ; p = reject ; sp=quarantine; rua=mailto:a@example.com, mailto:b@example.com ; ruf = mailto:c@example.com!10m ; adkim = s ; aspf = r ; ri= 3600 ; fo=1:d:s:0 ; rf=afrf:iodef ; pct= 75 ;")},
			want: &Record{
				Raw:   "v=DMARC1 ; p = reject ; sp=quarantine; rua=mailto:a@example.com, mailto:b@example.com ; ruf = mailto:c@example.com!10m ; adkim = s ; aspf = r ; ri= 3600 ; fo=1:d:s:0 ; rf=afrf:iodef ; pct= 75 ;",
				P:     "reject",
				Sp:    "quarantine",
				Rua:   "mailto:a@example.com, mailto:b@example.com",
				Ruf:   "mailto:c@example.com!10m",
				Adkim: "s",
				Aspf:  "r",
				Ri:    "3600",
				Fo:    "1:d:s:0",
				Rf:    "afrf:iodef",
				Pct:   "75",
			},
			wantErr: false,
			wantIs:  nil,
		},
		{
			name:    "duplicate key should be semantic error",
			args:    args{data: []byte("v=DMARC1; p=none; p=reject")},
			want:    nil,
			wantErr: true,
			wantIs:  motmedelErrors.ErrSemanticError,
		},
		{
			name:    "syntax error when version missing",
			args:    args{data: []byte("p=none; rua=mailto:a@example.com")},
			want:    nil,
			wantErr: true,
			wantIs:  motmedelErrors.ErrSyntaxError,
		},
		{
			name:    "syntax error when wrong version",
			args:    args{data: []byte("v=DMARC2; p=none")},
			want:    nil,
			wantErr: true,
			wantIs:  motmedelErrors.ErrSyntaxError,
		},
		{
			name: "quarantine with multiple rua/ruf, ri 86400, fo=0, rf=afrf, pct=0",
			args: args{data: []byte("v=DMARC1; p=quarantine; sp=none; rua=mailto:agg@example.com!100k, mailto:ops@example.org; ruf=mailto:forensic@example.com!1m,mailto:sec@example.net; ri=86400; fo=0; rf=afrf; pct=0")},
			want: &Record{
				Raw: "v=DMARC1; p=quarantine; sp=none; rua=mailto:agg@example.com!100k, mailto:ops@example.org; ruf=mailto:forensic@example.com!1m,mailto:sec@example.net; ri=86400; fo=0; rf=afrf; pct=0",
				P:   "quarantine",
				Sp:  "none",
				Rua: "mailto:agg@example.com!100k, mailto:ops@example.org",
				Ruf: "mailto:forensic@example.com!1m,mailto:sec@example.net",
				Ri:  "86400",
				Fo:  "0",
				Rf:  "afrf",
				Pct: "0",
			},
			wantErr: false,
			wantIs:  nil,
		},
		{
			name: "reject minimal with adkim=r aspf=s pct=100",
			args: args{data: []byte("v=DMARC1;p=reject;adkim=r;aspf=s;pct=100")},
			want: &Record{
				Raw:   "v=DMARC1;p=reject;adkim=r;aspf=s;pct=100",
				P:     "reject",
				Adkim: "r",
				Aspf:  "s",
				Pct:   "100",
			},
			wantErr: false,
			wantIs:  nil,
		},
		{
			name: "none with rf multi keywords, fo mix, and spaced rua list",
			args: args{data: []byte("v=DMARC1;rf=afrf:iodef:custom;fo=1:s;p=none;rua=mailto:a@ex.com ,mailto:b@ex.com")},
			want: &Record{
				Raw: "v=DMARC1;rf=afrf:iodef:custom;fo=1:s;p=none;rua=mailto:a@ex.com ,mailto:b@ex.com",
				P:   "none",
				Rua: "mailto:a@ex.com ,mailto:b@ex.com",
				Fo:  "1:s",
				Rf:  "afrf:iodef:custom",
			},
			wantErr: false,
			wantIs:  nil,
		},
		{
			name:    "unexpected key should be syntax error (unknown tag)",
			args:    args{data: []byte("v=DMARC1; p=none; x=abc")},
			want:    nil,
			wantErr: true,
			wantIs:  motmedelErrors.ErrSyntaxError,
		},
		{
			name:    "pct above 100 should be syntax error",
			args:    args{data: []byte("v=DMARC1; p=none; pct=101")},
			want:    nil,
			wantErr: true,
			wantIs:  motmedelErrors.ErrSyntaxError,
		},
		{
			name:    "pct three digits should be syntax error",
			args:    args{data: []byte("v=DMARC1; p=none; pct=999")},
			want:    nil,
			wantErr: true,
			wantIs:  motmedelErrors.ErrSyntaxError,
		},
		{
			name: "pct 0 is valid",
			args: args{data: []byte("v=DMARC1; p=none; pct=0")},
			want: &Record{
				Raw: "v=DMARC1; p=none; pct=0",
				P:   "none",
				Pct: "0",
			},
			wantErr: false,
			wantIs:  nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseDmarcRecord(tt.args.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseDmarcRecord() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantIs != nil {
				if !errors.Is(err, tt.wantIs) {
					t.Fatalf("ParseDmarcRecord() error = %v, wantIs %v", err, tt.wantIs)
				}
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseDmarcRecord() got = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestParseDmarcRecord_NilOrEmpty(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"nil", "empty"} {
		var input []byte
		if name == "empty" {
			input = []byte("")
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseDmarcRecord(input)
			if err == nil {
				t.Fatalf("expected error, got record=%v", got)
			}
			if !errors.Is(err, motmedelErrors.ErrSyntaxError) {
				t.Fatalf("expected ErrSyntaxError, got %v", err)
			}
		})
	}
}

func TestParseDmarcRecord_CanonicalizesCaseInsensitiveTags(t *testing.T) {
	t.Parallel()

	got, err := ParseDmarcRecord([]byte("v=DMARC1; P=REJECT; SP=Quarantine; ADKIM=S; ASPF=R; FO=D:S; RF=AFRF:IODEF"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := &Record{
		Raw:   "v=DMARC1; P=REJECT; SP=Quarantine; ADKIM=S; ASPF=R; FO=D:S; RF=AFRF:IODEF",
		P:     "reject",
		Sp:    "quarantine",
		Adkim: "s",
		Aspf:  "r",
		Fo:    "d:s",
		Rf:    "afrf:iodef",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v want %#v", got, want)
	}
}
