// To test another db backend:
// 1) Create GetAdapter function inside your db backend adapter package (like one inside mongodb adapter)
// 2) Uncomment your db backend package ('backend' named package)
// 3) Write own initConnectionToDb and 'db' variable
// 4) Replace mongodb specific db queries inside test to your own queries.
// 5) Run.

package tests

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"reflect"
	"testing"
	"time"

	jcr "github.com/DisposaBoy/JsonConfigReader"
	adapter "github.com/tinode/chat/server/db"

	b "go.mongodb.org/mongo-driver/bson"
	mdb "go.mongodb.org/mongo-driver/mongo"
	mdbopts "go.mongodb.org/mongo-driver/mongo/options"

	//backend "github.com/tinode/chat/server/db/rethinkdb"
	//backend "github.com/tinode/chat/server/db/mysql"
	backend "github.com/tinode/chat/server/db/mongodb"
	"github.com/tinode/chat/server/store/types"
)

type configType struct {
	// If Reset=true test will recreate database every time it runs
	Reset bool `json:"reset_db_data"`
	// Configurations for individual adapters.
	Adapters map[string]json.RawMessage `json:"adapters"`
}

var config configType
var adp adapter.Adapter
var db *mdb.Database
var ctx context.Context

func TestCreateDb(t *testing.T) {
	if err := adp.CreateDb(config.Reset); err != nil {
		t.Fatal(err)
	}
}

// ================== Create tests ================================
func TestUserCreate(t *testing.T) {
	for _, user := range users {
		if err := adp.UserCreate(user); err != nil {
			t.Error(err)
		}
	}
	count, err := db.Collection("users").CountDocuments(ctx, b.M{})
	if err != nil {
		t.Error(err)
	}
	if count == 0 {
		t.Error("No users created!")
	}
}

func TestCredUpsert(t *testing.T) {
	// Test just inserts:
	for i := 0; i < 2; i++ {
		inserted, err := adp.CredUpsert(creds[i])
		if err != nil {
			t.Fatal(err)
		}
		if !inserted {
			t.Error("Should be inserted, but updated")
		}
	}

	// Test duplicate:
	_, err := adp.CredUpsert(creds[1])
	if err != types.ErrDuplicate {
		t.Error("Should return duplicate error but got", err)
	}
	_, err = adp.CredUpsert(creds[2])
	if err != types.ErrDuplicate {
		t.Error("Should return duplicate error but got", err)
	}

	// Test add new unvalidated credentials
	inserted, err := adp.CredUpsert(creds[3])
	if err != nil {
		t.Fatal(err)
	}
	if !inserted {
		t.Error("Should be inserted, but updated")
	}
	inserted, err = adp.CredUpsert(creds[3])
	if err != nil {
		t.Fatal(err)
	}
	if inserted {
		t.Error("Should be updated, but inserted")
	}

	// Just insert other creds (used in other tests)
	for _, cred := range creds[4:] {
		_, err = adp.CredUpsert(cred)
		if err != nil {
			t.Fatal(err)
		}
	}
}

//func TestAuthAddRecord(t *testing.T) {
//	// TODO
//}
//
//func TestTopicCreate(t *testing.T) {
//	// TODO
//}
//
//func TestMessageSave(t *testing.T) {
//	// TODO
//}
//
//func TestDeviceUpsert(t *testing.T) {
//	// TODO
//}
//
//func TestFileStartUpload(t *testing.T) {
//	// TODO
//}

// ================== Read tests ==================================
func TestUserGet(t *testing.T) {
	// Test not found
	got, err := adp.UserGet(types.ParseUserId("dummyuserid"))
	if err == nil && got != nil {
		t.Error("user should be nil.")
	}

	got, err = adp.UserGet(types.ParseUserId("usr" + users[0].Id))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, users[0]) {
		t.Errorf(mismatchErrorString("User", got, users[0]))
	}
}

func TestUserGetAll(t *testing.T) {
	// Test not found
	got, err := adp.UserGetAll(types.ParseUserId("dummyuserid"), types.ParseUserId("otherdummyid"))
	if err == nil && got != nil {
		t.Error("result users should be nil.")
	}

	got, err = adp.UserGetAll(types.ParseUserId("usr"+users[0].Id), types.ParseUserId("usr"+users[1].Id))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatal(mismatchErrorString("resultUsers length", len(got), 2))
	}
	for i, usr := range got {
		if !reflect.DeepEqual(&usr, users[i]) {
			t.Error(mismatchErrorString("User", &usr, users[i]))
		}
	}
}

func TestUserGetDisabled(t *testing.T) {
	// Test before deletion date
	got, err := adp.UserGetDisabled(users[2].DeletedAt.Add(-10 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatal(mismatchErrorString("uids length", len(got), 1))
	}
	if got[0].String() != users[2].Id {
		t.Error(mismatchErrorString("userId", got[0].String(), users[2].Id))
	}

	// Test after deletion date
	got, err = adp.UserGetDisabled(users[2].DeletedAt.Add(10 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatal(mismatchErrorString("result", got, nil))
	}
}

func TestUserGetByCred(t *testing.T) {
	// Test not found
	got, err := adp.UserGetByCred("foo", "bar")
	if err != nil {
		t.Fatal(err)
	}
	if got != types.ZeroUid {
		t.Error("result uid should be ZeroUid")
	}

	got, err = adp.UserGetByCred(creds[0].Method, creds[0].Value)
	if got != types.ParseUserId("usr"+creds[0].User) {
		t.Error(mismatchErrorString("Uid", got, types.ParseUserId("usr"+creds[0].User)))
	}
}

func TestCredGetActive(t *testing.T) {
	got, err := adp.CredGetActive(types.ParseUserId("usr"+users[2].Id), "tel")
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(got, creds[3]) {
		t.Errorf(mismatchErrorString("Credential", got, creds[3]))
	}

	// Test not found
	_, err = adp.CredGetActive(types.ParseUserId("dummyusrid"), "")
	if err != types.ErrNotFound {
		t.Error("Err should be types.ErrNotFound, but got", err)
	}
}

func TestCredGetAll(t *testing.T) {
	got, err := adp.CredGetAll(types.ParseUserId("usr"+users[2].Id), "", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf(mismatchErrorString("Credentials length", len(got), 2))
	}

	got, err = adp.CredGetAll(types.ParseUserId("usr"+users[2].Id), "tel", false)
	if len(got) != 2 {
		t.Errorf(mismatchErrorString("Credentials length", len(got), 2))
	}

	got, err = adp.CredGetAll(types.ParseUserId("usr"+users[2].Id), "", true)
	if len(got) != 1 {
		t.Errorf(mismatchErrorString("Credentials length", len(got), 1))
	}

	got, err = adp.CredGetAll(types.ParseUserId("usr"+users[2].Id), "tel", true)
	if len(got) != 1 {
		t.Errorf(mismatchErrorString("Credentials length", len(got), 1))
	}
}

//func TestUserUnreadCount(t *testing.T) {
//	// TODO
//}
//
//
//
//func TestAuthGetUniqueRecord(t *testing.T) {
//	// TODO
//}
//
//func TestAuthGetRecord(t *testing.T) {
//	// TODO
//}
//
//func TestTopicGet(t *testing.T) {
//	// TODO
//}
//
//func TestTopicsForUser(t *testing.T) {
//	// TODO
//}
//
//func TestUsersForTopic(t *testing.T) {
//	// TODO
//}
//
//func TestOwnTopics(t *testing.T) {
//	// TODO
//}
//
//func TestSubscriptionGet(t *testing.T) {
//	// TODO
//}
//
//func TestSubsForUser(t *testing.T) {
//	// TODO
//}
//
//func TestSubsForTopic(t *testing.T) {
//	// TODO
//}
//
//func TestFindUsers(t *testing.T) {
//	// TODO
//}
//
//func TestFindTopics(t *testing.T) {
//	// TODO
//}
//
//func TestMessageGetAll(t *testing.T) {
//	// TODO
//}
//
//func TestMessageGetDeleted(t *testing.T) {
//	// TODO
//}
//
//func TestDeviceGetAll(t *testing.T) {
//	// TODO
//}
//
//func TestFileGet(t *testing.T) {
//	// TODO
//}

// ================== Update tests ================================
//func TestUserUpdate(t *testing.T) {
//	// TODO
//}
//
//func TestUserUpdateTags(t *testing.T) {
//	// TODO
//}
//
//func TestCredFail(t *testing.T) {
//	// TODO
//}
//
//func TestAuthUpdRecord(t *testing.T) {
//	// TODO
//}
//
//func TestTopicUpdateOnMessage(t *testing.T) {
//	// TODO
//}
//
//func TestTopicUpdate(t *testing.T) {
//	// TODO
//}
//
//func TestTopicOwnerChange(t *testing.T) {
//	// TODO
//}
//
//func TestSubsUpdate(t *testing.T) {
//	// TODO
//}
//
//func TestMessageAttachments(t *testing.T) {
//	// TODO
//}
//
//func TestDeviceDelete(t *testing.T) {
//	// TODO
//}
//
//func TestFileFinishUpload(t *testing.T) {
//	// TODO
//}

// ================== Delete tests ================================
//func TestUserDelete(t *testing.T) {
//	// TODO
//}
//
//func TestCredDel(t *testing.T) {
//	// TODO
//}
//
//func TestAuthDelScheme(t *testing.T) {
//	// TODO
//}
//
//func TestAuthDelAllRecords(t *testing.T) {
//	// TODO
//}
//
//func TestTopicDelete(t *testing.T) {
//	// TODO
//}
//
//func TestSubsDelete(t *testing.T) {
//	// TODO
//}
//
//func TestFileDeleteUnused(t *testing.T) {
//	// TODO
//}
//
//// ================== Mixed tests =================================
//func TestCredConfirm(t *testing.T) {
//	// TODO
//}
//
//func TestTopicCreateP2P(t *testing.T) {
//	// TODO
//}
//
//func TestTopicShare(t *testing.T) {
//	// TODO
//}
//
//func TestSubsDelForTopic(t *testing.T) {
//	// TODO
//}
//
//func TestSubsDelForUser(t *testing.T) {
//	// TODO
//}
//
//func TestMessageDeleteList(t *testing.T) {
//	// TODO
//}

// ================================================================
func mismatchErrorString(key string, got, want interface{}) string {
	return fmt.Sprintf("%v mismatch:\nGot  = %v\nWant = %v", key, got, want)
}

func initConnectionToDb() {
	var adpConfig struct {
		Addresses interface{} `json:"addresses,omitempty"`
		Database  string      `json:"database,omitempty"`
	}

	if err := json.Unmarshal(config.Adapters[adp.GetName()], &adpConfig); err != nil {
		log.Fatal("adapter mongodb failed to parse config: " + err.Error())
	}

	var opts mdbopts.ClientOptions

	if adpConfig.Addresses == nil {
		opts.SetHosts([]string{"localhost:27017"})
	} else if host, ok := adpConfig.Addresses.(string); ok {
		opts.SetHosts([]string{host})
	} else if hosts, ok := adpConfig.Addresses.([]string); ok {
		opts.SetHosts(hosts)
	} else {
		log.Fatal("adapter mongodb failed to parse config.Addresses")
	}

	if adpConfig.Database == "" {
		adpConfig.Database = "tinode_test"
	}

	ctx = context.Background()
	conn, err := mdb.Connect(ctx, &opts)
	if err != nil {
		log.Fatal(err)
	}

	db = conn.Database(adpConfig.Database)
}

func init() {
	adp = backend.GetAdapter()
	conffile := flag.String("config", "./test.conf", "config of the database connection")

	if file, err := os.Open(*conffile); err != nil {
		log.Fatal("Failed to read config file:", err)
	} else if err = json.NewDecoder(jcr.New(file)).Decode(&config); err != nil {
		log.Fatal("Failed to parse config file:", err)
	}

	if adp == nil {
		log.Fatal("Database adapter is missing")
	}
	if adp.IsOpen() {
		log.Print("Connection is already opened")
	}

	err := adp.Open(config.Adapters[adp.GetName()])
	if err != nil {
		log.Fatal(err)
	}

	initConnectionToDb()
	initData()
}
