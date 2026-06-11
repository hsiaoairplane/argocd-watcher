package main

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/cache"
)

func TestAppKey(t *testing.T) {
	tests := []struct {
		name    string
		obj     map[string]interface{}
		want    string
		wantErr bool
	}{
		{
			name: "project and name present",
			obj: map[string]interface{}{
				"metadata": map[string]interface{}{"name": "my-app"},
				"spec":     map[string]interface{}{"project": "my-proj"},
			},
			want: "my-proj|my-app",
		},
		{
			name: "missing project yields empty project segment",
			obj: map[string]interface{}{
				"metadata": map[string]interface{}{"name": "my-app"},
			},
			want: "|my-app",
		},
		{
			name: "wrong type for project returns error",
			obj: map[string]interface{}{
				"metadata": map[string]interface{}{"name": "my-app"},
				"spec":     map[string]interface{}{"project": 123},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &unstructured.Unstructured{Object: tt.obj}
			got, err := appKey(u)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("appKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestToUnstructured(t *testing.T) {
	u := &unstructured.Unstructured{Object: map[string]interface{}{"kind": "Application"}}

	t.Run("plain object", func(t *testing.T) {
		got, ok := toUnstructured(u)
		if !ok || got != u {
			t.Fatalf("toUnstructured(u) = %v, %v; want %v, true", got, ok, u)
		}
	})

	t.Run("tombstone", func(t *testing.T) {
		tombstone := cache.DeletedFinalStateUnknown{Key: "argocd/my-app", Obj: u}
		got, ok := toUnstructured(tombstone)
		if !ok || got != u {
			t.Fatalf("toUnstructured(tombstone) = %v, %v; want %v, true", got, ok, u)
		}
	})

	t.Run("unexpected type", func(t *testing.T) {
		if _, ok := toUnstructured("not-an-object"); ok {
			t.Fatalf("toUnstructured(string) ok = true; want false")
		}
	})
}
