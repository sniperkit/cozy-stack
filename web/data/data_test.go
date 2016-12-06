package data

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/cozy/checkup"
	"github.com/cozy/cozy-stack/config"
	"github.com/cozy/cozy-stack/couchdb"
	"github.com/cozy/cozy-stack/instance"
	"github.com/cozy/cozy-stack/web"
	"github.com/labstack/echo"
	"github.com/stretchr/testify/assert"
)

var client = &http.Client{}

const Host = "example.com"
const Type = "io.cozy.events"
const ID = "4521C325F6478E45"
const ExpectedDBName = "example-com%2Fio-cozy-events"

var testInstance = &instance.Instance{Domain: "example.com"}

var ts *httptest.Server

// @TODO this should be moved to our couchdb package or to
// some test helpers files.

type stackUpdateResponse struct {
	ID      string          `json:"id"`
	Rev     string          `json:"rev"`
	Type    string          `json:"type"`
	Ok      bool            `json:"ok"`
	Deleted bool            `json:"deleted"`
	Error   string          `json:"error"`
	Reason  string          `json:"reason"`
	Data    couchdb.JSONDoc `json:"data"`
}

func jsonReader(data *map[string]interface{}) io.Reader {
	bs, _ := json.Marshal(&data)
	return bytes.NewReader(bs)
}

func docURL(ts *httptest.Server, doc couchdb.JSONDoc) string {
	return ts.URL + "/data/" + doc.DocType() + "/" + doc.ID()
}

func doRequest(req *http.Request, out interface{}) (jsonres map[string]interface{}, res *http.Response, err error) {
	res, err = client.Do(req)
	if err != nil {
		return
	}
	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return
	}
	if out == nil {
		var out map[string]interface{}
		err = json.Unmarshal(body, &out)
		if err != nil {
			return
		}
		return out, res, err
	}
	err = json.Unmarshal(body, &out)
	if err != nil {
		return
	}
	return nil, res, err

}

func injectInstance(i *instance.Instance) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("instance", i)
			return next(c)
		}
	}
}

func getDocForTest() couchdb.JSONDoc {
	doc := couchdb.JSONDoc{Type: Type, M: map[string]interface{}{"test": "value"}}
	couchdb.CreateDoc(testInstance, &doc)
	return doc
}

func TestMain(m *testing.M) {
	config.UseTestFile()

	db, err := checkup.HTTPChecker{URL: config.CouchURL()}.Check()
	if err != nil || db.Status() != checkup.Healthy {
		fmt.Println("This test need couchdb to run.")
		os.Exit(1)
	}

	instance.Destroy(Host)

	inst, err := instance.Create(Host, "en", nil)
	if err != nil {
		fmt.Println("Could not create test instance.", err)
		os.Exit(1)
	}

	handler := echo.New()
	handler.HTTPErrorHandler = web.ErrorHandler
	Routes(handler.Group("/data", injectInstance(inst)))
	ts = httptest.NewServer(handler)

	couchdb.ResetDB(testInstance, Type)
	doc := couchdb.JSONDoc{Type: Type, M: map[string]interface{}{
		"_id":  ID,
		"test": "testvalue",
	}}
	couchdb.CreateNamedDoc(testInstance, &doc)

	res := m.Run()

	ts.Close()
	instance.Destroy(Host)

	os.Exit(res)
}

func TestSuccessGet(t *testing.T) {
	req, _ := http.NewRequest("GET", ts.URL+"/data/"+Type+"/"+ID, nil)
	req.Header.Add("Host", Host)
	out, res, err := doRequest(req, nil)
	assert.NoError(t, err)
	assert.Equal(t, "200 OK", res.Status, "should get a 200")
	if assert.Contains(t, out, "test") {
		assert.Equal(t, out["test"], "testvalue", "should give the same doc")
	}
}

func TestWrongDoctype(t *testing.T) {

	couchdb.DeleteDB(testInstance, "nottype")

	req, _ := http.NewRequest("GET", ts.URL+"/data/nottype/"+ID, nil)
	req.Header.Add("Host", Host)
	out, res, err := doRequest(req, nil)
	assert.NoError(t, err)
	assert.Equal(t, "404 Not Found", res.Status, "should get a 404")
	out = out["errors"].([]interface{})[0].(map[string]interface{})
	if assert.Contains(t, out, "title") {
		assert.Equal(t, "not_found", out["title"], "should give a json title")
	}
	if assert.Contains(t, out, "detail") {
		assert.Equal(t, "wrong_doctype", out["detail"], "should give a detail")
	}

}

func TestVFSDoctype(t *testing.T) {

	var in = jsonReader(&map[string]interface{}{
		"wrong-vfs": "structure",
	})
	req, _ := http.NewRequest("POST", ts.URL+"/data/io.cozy.files/", in)
	req.Header.Add("Host", Host)
	req.Header.Set("Content-Type", "application/json")
	out, res, err := doRequest(req, nil)
	assert.NoError(t, err)
	assert.Equal(t, "403 Forbidden", res.Status, "should get a 403")
	out = out["errors"].([]interface{})[0].(map[string]interface{})
	if assert.Contains(t, out, "detail") {
		assert.Contains(t, out["detail"], "reserved", "should give a clear detail")
	}
}

func TestWrongID(t *testing.T) {
	req, _ := http.NewRequest("GET", ts.URL+"/data/"+Type+"/NOTID", nil)
	req.Header.Add("Host", Host)
	out, res, err := doRequest(req, nil)
	assert.NoError(t, err)
	assert.Equal(t, "404 Not Found", res.Status, "should get a 404")
	out = out["errors"].([]interface{})[0].(map[string]interface{})
	if assert.Contains(t, out, "title") {
		assert.Equal(t, "not_found", out["title"], "should give a json title")
	}
	if assert.Contains(t, out, "detail") {
		assert.Equal(t, "missing", out["detail"], "should give a detail")
	}
}

func TestWrongHost(t *testing.T) {
	t.Skip("unskip me when we stop falling back to Host = dev")
	req, _ := http.NewRequest("GET", ts.URL+"/data/"+Type+"/"+ID, nil)
	req.Header.Add("Host", "NOTHOST")
	out, res, err := doRequest(req, nil)
	assert.NoError(t, err)
	assert.Equal(t, "404 Not Found", res.Status, "should get a 404")
	out = out["errors"].([]interface{})[0].(map[string]interface{})
	if assert.Contains(t, out, "title") {
		assert.Equal(t, "not_found", out["title"], "should give a json title")
	}
	if assert.Contains(t, out, "detail") {
		assert.Equal(t, "wrong_doctype", out["detail"], "should give a detail")
	}
}

func TestSuccessCreateKnownDoctype(t *testing.T) {
	var in = jsonReader(&map[string]interface{}{
		"somefield": "avalue",
	})
	var sur stackUpdateResponse
	req, _ := http.NewRequest("POST", ts.URL+"/data/"+Type+"/", in)
	req.Header.Add("Host", Host)
	req.Header.Set("Content-Type", "application/json")
	_, res, err := doRequest(req, &sur)
	assert.NoError(t, err)
	assert.Equal(t, "201 Created", res.Status, "should get a 201")
	assert.Equal(t, sur.Ok, true, "ok is true")
	assert.NotContains(t, sur.ID, "/", "id is simple uuid")
	assert.Equal(t, sur.Type, Type, "type is correct")
	assert.NotEmpty(t, sur.Rev, "rev at top level (couchdb compatibility)")
	assert.Equal(t, sur.ID, sur.Data.ID(), "id is simple uuid")
	assert.Equal(t, sur.Type, sur.Data.Type, "type is correct")
	assert.Equal(t, sur.Rev, sur.Data.Rev(), "rev is correct")
	assert.Equal(t, "avalue", sur.Data.Get("somefield"), "content is correct")
}

func TestSuccessCreateUnknownDoctype(t *testing.T) {
	var in = jsonReader(&map[string]interface{}{
		"somefield": "avalue",
	})
	var sur stackUpdateResponse
	type2 := "io.cozy.anothertype"
	req, _ := http.NewRequest("POST", ts.URL+"/data/"+type2+"/", in)
	req.Header.Add("Host", Host)
	req.Header.Set("Content-Type", "application/json")
	_, res, err := doRequest(req, &sur)
	assert.NoError(t, err)
	assert.Equal(t, "201 Created", res.Status, "should get a 201")
	assert.Equal(t, sur.Ok, true, "ok is true")
	assert.NotContains(t, sur.ID, "/", "id is simple uuid")
	assert.Equal(t, sur.Type, type2, "type is correct")
	assert.NotEmpty(t, sur.Rev, "rev at top level (couchdb compatibility)")
	assert.Equal(t, sur.ID, sur.Data.ID(), "in doc id is correct")
	assert.Equal(t, sur.Type, sur.Data.Type, "in doc type is correct")
	assert.Equal(t, sur.Rev, sur.Data.Rev(), "in doc rev is correct")
	assert.Equal(t, "avalue", sur.Data.Get("somefield"), "content is correct")
}

func TestWrongCreateWithID(t *testing.T) {
	var in = jsonReader(&map[string]interface{}{
		"_id":       "this-should-not-be-an-id",
		"somefield": "avalue",
	})
	req, _ := http.NewRequest("POST", ts.URL+"/data/"+Type+"/", in)
	req.Header.Add("Host", Host)
	req.Header.Set("Content-Type", "application/json")
	_, res, err := doRequest(req, nil)
	assert.NoError(t, err)
	assert.Equal(t, "400 Bad Request", res.Status, "should get a 400")
}

func TestSuccessUpdate(t *testing.T) {

	// Get revision
	doc := getDocForTest()
	url := ts.URL + "/data/" + doc.DocType() + "/" + doc.ID()

	// update it
	var in = jsonReader(&map[string]interface{}{
		"_id":       doc.ID(),
		"_rev":      doc.Rev(),
		"test":      doc.Get("test"),
		"somefield": "anewvalue",
	})
	req, _ := http.NewRequest("PUT", url, in)
	req.Header.Add("Host", Host)
	req.Header.Set("Content-Type", "application/json")
	var out stackUpdateResponse
	_, res, err := doRequest(req, &out)
	assert.NoError(t, err)
	assert.Equal(t, "200 OK", res.Status, "should get a 201")
	assert.Empty(t, out.Error, "there is no error")
	assert.Equal(t, out.ID, doc.ID(), "id has not changed")
	assert.Equal(t, out.Ok, true, "ok is true")
	assert.NotEmpty(t, out.Rev, "there is a rev")
	assert.NotEqual(t, out.Rev, doc.Rev(), "rev has changed")
	assert.Equal(t, out.ID, out.Data.ID(), "in doc id is simple uuid")
	assert.Equal(t, out.Type, out.Data.Type, "in doc type is correct")
	assert.Equal(t, out.Rev, out.Data.Rev(), "in doc rev is correct")
	assert.Equal(t, "anewvalue", out.Data.Get("somefield"), "content has changed")
}

// Test for having not the same ID in document and URL
func TestWrongIDInDocUpdate(t *testing.T) {
	// Get revision
	doc := getDocForTest()
	// update it
	var in = jsonReader(&map[string]interface{}{
		"_id":       "this is not the id in the URL",
		"_rev":      doc.Rev(),
		"test":      doc.M["test"],
		"somefield": "anewvalue",
	})
	url := ts.URL + "/data/" + doc.DocType() + "/" + doc.ID()
	req, _ := http.NewRequest("PUT", url, in)
	req.Header.Add("Host", Host)
	req.Header.Set("Content-Type", "application/json")
	_, res, err := doRequest(req, nil)
	assert.NoError(t, err)
	assert.Equal(t, "400 Bad Request", res.Status, "should get a 404")
}

// Test for having an inexisting id at all
func TestCreateDocWithAFixedID(t *testing.T) {
	// update it
	var in = jsonReader(&map[string]interface{}{
		"test":      "value",
		"somefield": "anewvalue",
	})
	url := ts.URL + "/data/" + Type + "/specific-id"
	req, _ := http.NewRequest("PUT", url, in)
	req.Header.Add("Host", Host)
	req.Header.Set("Content-Type", "application/json")
	var out stackUpdateResponse
	_, res, err := doRequest(req, &out)
	assert.NoError(t, err)
	assert.Equal(t, "200 OK", res.Status, "should get a 201")
	assert.Empty(t, out.Error, "there is no error")
	assert.Equal(t, out.ID, "specific-id", "id has not changed")
	assert.Equal(t, out.Ok, true, "ok is true")
	assert.NotEmpty(t, out.Rev, "there is a rev")
	assert.Equal(t, out.ID, out.Data.ID(), "in doc id is simple uuid")
	assert.Equal(t, out.Type, out.Data.Type, "in doc type is correct")
	assert.Equal(t, out.Rev, out.Data.Rev(), "in doc rev is correct")
	assert.Equal(t, "anewvalue", out.Data.Get("somefield"), "content has changed")

}

func TestNoRevInDocUpdate(t *testing.T) {
	// Get revision
	doc := getDocForTest()
	// update it
	var in = jsonReader(&map[string]interface{}{
		"_id":       doc.ID(),
		"test":      doc.M["test"],
		"somefield": "anewvalue",
	})
	url := ts.URL + "/data/" + doc.DocType() + "/" + doc.ID()
	req, _ := http.NewRequest("PUT", url, in)
	req.Header.Add("Host", Host)
	req.Header.Set("Content-Type", "application/json")
	_, res, err := doRequest(req, nil)
	assert.NoError(t, err)
	assert.Equal(t, "400 Bad Request", res.Status, "should get a 400")
}

func TestPreviousRevInDocUpdate(t *testing.T) {
	// Get revision
	doc := getDocForTest()
	firstRev := doc.Rev()
	url := ts.URL + "/data/" + doc.DocType() + "/" + doc.ID()

	// correcly update it
	var in = jsonReader(&map[string]interface{}{
		"_id":       doc.ID(),
		"_rev":      doc.Rev(),
		"somefield": "anewvalue",
	})
	req, _ := http.NewRequest("PUT", url, in)
	req.Header.Add("Host", Host)
	req.Header.Set("Content-Type", "application/json")
	_, res, err := doRequest(req, nil)
	assert.NoError(t, err)
	assert.Equal(t, "200 OK", res.Status, "first update should work")

	// update it
	var in2 = jsonReader(&map[string]interface{}{
		"_id":       doc.ID(),
		"_rev":      firstRev,
		"somefield": "anewvalue2",
	})
	req2, _ := http.NewRequest("PUT", url, in2)
	req2.Header.Add("Host", Host)
	req2.Header.Set("Content-Type", "application/json")
	_, res2, err := doRequest(req2, nil)
	assert.NoError(t, err)
	assert.Equal(t, "409 Conflict", res2.Status, "should get a 409")
}

func TestSuccessDeleteIfMatch(t *testing.T) {
	// Get revision
	doc := getDocForTest()
	rev := doc.Rev()

	// Do deletion
	url := ts.URL + "/data/" + doc.DocType() + "/" + doc.ID()
	req, _ := http.NewRequest("DELETE", url, nil)
	req.Header.Add("If-Match", rev)
	req.Header.Add("Host", Host)
	var out stackUpdateResponse
	_, res, err := doRequest(req, &out)
	assert.NoError(t, err)
	assert.Equal(t, "200 OK", res.Status, "should get a 201")
	assert.Equal(t, out.Ok, true, "ok at top level (couchdb compatibility)")
	assert.Equal(t, out.ID, doc.ID(), "id at top level (couchdb compatibility)")
	assert.Equal(t, out.Deleted, true, "id at top level (couchdb compatibility)")
	assert.NotEqual(t, out.Rev, doc.Rev(), "id at top level (couchdb compatibility)")
}

func TestFailDeleteIfNotMatch(t *testing.T) {
	// Get revision
	doc := getDocForTest()

	// Do deletion
	url := ts.URL + "/data/" + doc.DocType() + "/" + doc.ID()
	req, _ := http.NewRequest("DELETE", url, nil)
	req.Header.Add("If-Match", "1-238238232322121") // not correct rev
	req.Header.Add("Host", Host)
	var out stackUpdateResponse
	_, res, err := doRequest(req, &out)
	assert.NoError(t, err)
	assert.Equal(t, "409 Conflict", res.Status, "should get a 409")
}

func TestFailDeleteIfHeaderAndRevMismatch(t *testing.T) {
	// Get revision
	doc := getDocForTest()

	// Do deletion
	url := ts.URL + "/data/" + doc.DocType() + "/" + doc.ID() + "?rev=1-238238232322121"
	req, _ := http.NewRequest("DELETE", url, nil)
	req.Header.Add("If-Match", "1-23823823231") // not same rev
	req.Header.Add("Host", Host)
	var out stackUpdateResponse
	_, res, err := doRequest(req, &out)
	assert.NoError(t, err)
	assert.Equal(t, "400 Bad Request", res.Status, "should get a 400")
}

func TestFailDeleteIfNoRev(t *testing.T) {
	// Get revision
	doc := getDocForTest()

	// Do deletion
	url := ts.URL + "/data/" + doc.DocType() + "/" + doc.ID()
	req, _ := http.NewRequest("DELETE", url, nil)
	req.Header.Add("Host", Host)
	var out stackUpdateResponse
	_, res, err := doRequest(req, &out)
	assert.NoError(t, err)
	assert.Equal(t, "400 Bad Request", res.Status, "should get a 400")
}

type M map[string]interface{}
type S []interface{}
type indexCreationResponse struct {
	Result string `json:"result"`
	Error  string `json:"error"`
	Reason string `json:"reason"`
	ID     string `json:"id"`
	Name   string `json:"name"`
}

func TestDefineIndex(t *testing.T) {
	var def map[string]interface{}
	def = M{"index": M{"fields": S{"foo"}}}
	var url = ts.URL + "/data/" + Type + "/_index"
	req, _ := http.NewRequest("POST", url, jsonReader(&def))
	req.Header.Add("Host", Host)
	req.Header.Set("Content-Type", "application/json")
	var out indexCreationResponse
	_, res, err := doRequest(req, &out)
	assert.NoError(t, err)
	assert.Equal(t, "200 OK", res.Status, "first update should work")
	assert.Empty(t, out.Error, "should have no error")
	assert.Empty(t, out.Reason, "should have no error")
	assert.Equal(t, "created", out.Result, "should have created result")
	assert.NotEmpty(t, out.Name, "should have a name")
	assert.NotEmpty(t, out.ID, "should have an design doc ID")
}

func TestReDefineIndex(t *testing.T) {
	var def map[string]interface{}
	def = M{"index": M{"fields": S{"foo"}}}
	var url = ts.URL + "/data/" + Type + "/_index"
	req, _ := http.NewRequest("POST", url, jsonReader(&def))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Add("Host", Host)
	var out indexCreationResponse
	_, res, err := doRequest(req, &out)
	assert.NoError(t, err)
	assert.Equal(t, "200 OK", res.Status)
	assert.Empty(t, out.Error, "should have no error")
	assert.Empty(t, out.Reason, "should have no error")
	assert.Equal(t, "exists", out.Result, "should have exists result")
	assert.NotEmpty(t, out.Name, "should have a name")
	assert.NotEmpty(t, out.ID, "should have an design doc ID")
}

func TestDefineIndexUnexistingDoctype(t *testing.T) {

	couchdb.DeleteDB(testInstance, "nottype")

	var def map[string]interface{}
	def = M{"index": M{"fields": S{"foo"}}}
	var url = ts.URL + "/data/nottype/_index"
	req, _ := http.NewRequest("POST", url, jsonReader(&def))
	req.Header.Add("Host", Host)
	req.Header.Set("Content-Type", "application/json")
	var out indexCreationResponse
	_, res, err := doRequest(req, &out)
	assert.NoError(t, err)
	assert.Equal(t, "200 OK", res.Status)
	assert.Empty(t, out.Error, "should have no error")
	assert.Empty(t, out.Reason, "should have no error")
	assert.Equal(t, "created", out.Result, "should have created result")
	assert.NotEmpty(t, out.Name, "should have a name")
	assert.NotEmpty(t, out.ID, "should have an design doc ID")
}

func TestFindDocuments(t *testing.T) {

	couchdb.ResetDB(testInstance, Type)

	_ = getDocForTest()
	_ = getDocForTest()
	_ = getDocForTest()

	var def map[string]interface{}
	def = M{"index": M{"fields": S{"test"}}}
	var url = ts.URL + "/data/" + Type + "/_index"
	req, _ := http.NewRequest("POST", url, jsonReader(&def))
	req.Header.Add("Host", Host)
	req.Header.Set("Content-Type", "application/json")
	var out indexCreationResponse
	_, _, err := doRequest(req, &out)
	assert.NoError(t, err)
	assert.Empty(t, out.Error, "should have no error")

	var query map[string]interface{}
	query = M{"selector": M{"test": "value"}}
	var url2 = ts.URL + "/data/" + Type + "/_find"
	req, _ = http.NewRequest("POST", url2, jsonReader(&query))
	req.Header.Add("Host", Host)
	req.Header.Set("Content-Type", "application/json")
	var out2 struct {
		Docs []couchdb.JSONDoc `json:"docs"`
	}
	_, res, err := doRequest(req, &out2)
	assert.Equal(t, "200 OK", res.Status, "should get a 200")
	assert.NoError(t, err)
	assert.Len(t, out2.Docs, 3, "should have found 3 docs")
}

func TestFindDocumentsWithoutIndex(t *testing.T) {
	var query map[string]interface{}
	query = M{"selector": M{"no-index-for-this-field": "value"}}
	var url2 = ts.URL + "/data/" + Type + "/_find"
	req, _ := http.NewRequest("POST", url2, jsonReader(&query))
	req.Header.Add("Host", Host)
	req.Header.Set("Content-Type", "application/json")
	var out2 struct {
		Errors []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		} `json:"errors"`
	}
	_, res, err := doRequest(req, &out2)
	assert.Equal(t, "400 Bad Request", res.Status, "should get a 200")
	assert.NoError(t, err)
	assert.Contains(t, out2.Errors[0].Title, "no_index")
	assert.Contains(t, out2.Errors[0].Detail, "no matching index")
}
