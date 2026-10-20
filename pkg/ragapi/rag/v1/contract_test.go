package ragv1_test

import (
	"testing"

	"google.golang.org/protobuf/proto"

	ragv1 "github.com/ivanSaichkin/rag-movie-picker/pkg/ragapi/rag/v1"
)

func TestOptionalFieldPresence(t *testing.T) {
	request := &ragv1.RecommendRequest{
		Query: "space",
		Filters: &ragv1.Filters{
			MinRating: proto.Float64(0),
		},
	}
	data, err := proto.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	var decoded ragv1.RecommendRequest
	if err := proto.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Limit != nil {
		t.Fatal("omitted limit must stay absent")
	}
	if decoded.Filters.MinRating == nil || *decoded.Filters.MinRating != 0 {
		t.Fatal("explicit zero rating must preserve presence")
	}
	if decoded.Filters.YearFrom != nil {
		t.Fatal("omitted year must stay absent")
	}
}

func TestServiceContract(t *testing.T) {
	service := ragv1.File_rag_v1_rag_proto.Services().ByName("RagService")
	if service == nil || service.Methods().Len() != 3 {
		t.Fatal("expected Recommend, GetMovie and GetCatalog")
	}
	for _, name := range []string{"Recommend", "GetMovie", "GetCatalog"} {
		found := false
		for i := 0; i < service.Methods().Len(); i++ {
			found = found || string(service.Methods().Get(i).Name()) == name
		}
		if !found {
			t.Fatalf("missing RPC %s", name)
		}
	}
}
