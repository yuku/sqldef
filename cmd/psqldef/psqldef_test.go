// Integration test of psqldef command.
//
// Test requirement:
//   - go command
//   - `psql -Upostgres` must succeed
package main

import (
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

const (
	applyPrefix     = "-- Apply --\n"
	nothingModified = "-- Nothing is modified --\n"
)

func TestPsqldefCreateTable(t *testing.T) {
	resetTestDatabase()

	createTable1 := stripHeredoc(`
		CREATE TABLE users (
		  id bigint NOT NULL,
		  name text,
		  age integer
		);
		`,
	)
	createTable2 := stripHeredoc(`
		CREATE TABLE bigdata (
		  data bigint
		);
		`,
	)

	assertApplyOutput(t, createTable1+createTable2, applyPrefix+createTable1+createTable2)
	assertApplyOutput(t, createTable1+createTable2, nothingModified)

	assertApplyOutput(t, createTable1, applyPrefix+"DROP TABLE \"public\".\"bigdata\";\n")
	assertApplyOutput(t, createTable1, nothingModified)
}

func TestPsqldefCreateTableWithDefault(t *testing.T) {
	resetTestDatabase()

	createTable := stripHeredoc(`
		CREATE TABLE users (
		  profile character varying(50) NOT NULL DEFAULT ''::character varying,
		  default_int int default 20,
		  default_bool bool default true,
		  default_numeric numeric(5) default 42.195,
		  default_fixed_char character(3) default 'JPN'::bpchar,
		  default_text text default ''::text,
		  default_json json default '[]'::json,
		  default_current_timestamp timestamp default CURRENT_TIMESTAMP,
		  default_current_date date default CURRENT_DATE,
		  default_current_time time default CURRENT_TIME,
		  joined_at timestamp with time zone NOT NULL DEFAULT '0001-01-01 00:00:00'::timestamp without time zone,
		  created_at timestamp with time zone DEFAULT now()
		);
		`,
	)
	assertApplyOutput(t, createTable, applyPrefix+createTable)
	assertApplyOutput(t, createTable, nothingModified)
}

func TestPsqldefCreateTableChangeDefaultBoolean(t *testing.T) {
	resetTestDatabase()

	createTable := stripHeredoc(`
		CREATE TABLE test (
		  col boolean default true
		);
		`,
	)
	assertApplyOutput(t, createTable, applyPrefix+createTable)
	assertApplyOutput(t, createTable, nothingModified)

	createTable = stripHeredoc(`
		CREATE TABLE test (
		  col boolean default false
		);
		`,
	)
	assertApplyOutput(t, createTable, applyPrefix+stripHeredoc(`
		ALTER TABLE "public"."test" ALTER COLUMN "col" SET DEFAULT false;
		`,
	))
	assertApplyOutput(t, createTable, nothingModified)
}

func TestPsqldefCreateTableChangeDefaultTimestamp(t *testing.T) {
	resetTestDatabase()

	createTable := stripHeredoc(`
		CREATE TABLE timestamps (
		  created_at timestamp default current_timestamp
		);
		`,
	)
	assertApplyOutput(t, createTable, applyPrefix+createTable)
	assertApplyOutput(t, createTable, nothingModified)

	createTableDropDefault := stripHeredoc(`
		CREATE TABLE timestamps (
		  created_at timestamp
		);
		`,
	)
	alter1 := `ALTER TABLE "public"."timestamps" ALTER COLUMN "created_at" DROP DEFAULT;`
	assertApplyOutput(t, createTableDropDefault, applyPrefix+alter1+"\n")
	assertApplyOutput(t, createTableDropDefault, nothingModified)

	alter2 := `ALTER TABLE "public"."timestamps" ALTER COLUMN "created_at" SET DEFAULT current_timestamp;`
	assertApplyOutput(t, createTable, applyPrefix+alter2+"\n")
	assertApplyOutput(t, createTable, nothingModified)
}

func TestPsqldefCreateTableAlterColumn(t *testing.T) {
	resetTestDatabase()

	createTable := stripHeredoc(`
		CREATE TABLE users (
		  id bigint NOT NULL,
		  name text
		);
		`,
	)
	assertApplyOutput(t, createTable, applyPrefix+createTable)
	assertApplyOutput(t, createTable, nothingModified)

	createTable = stripHeredoc(`
		CREATE TABLE users (
		  id bigint NOT NULL,
		  name varchar(40)
		);
		`,
	)
	assertApplyOutput(t, createTable, applyPrefix+stripHeredoc(`
		ALTER TABLE "public"."users" ALTER COLUMN "name" TYPE varchar(40);
		`,
	))
	assertApplyOutput(t, createTable, nothingModified)
}

func TestPsqldefCreateTableNotNull(t *testing.T) {
	resetTestDatabase()

	createTable := stripHeredoc(`
		CREATE TABLE users (
		  name text
		);
		`,
	)
	assertApplyOutput(t, createTable, applyPrefix+createTable)
	assertApplyOutput(t, createTable, nothingModified)

	createTable = stripHeredoc(`
		CREATE TABLE users (
		  name text NOT NULL
		);
		`,
	)
	assertApplyOutput(t, createTable, applyPrefix+stripHeredoc(`
		ALTER TABLE "public"."users" ALTER COLUMN "name" SET NOT NULL;
		`,
	))
	assertApplyOutput(t, createTable, nothingModified)

	createTable = stripHeredoc(`
		CREATE TABLE users (
		  name text
		);
		`,
	)
	assertApplyOutput(t, createTable, applyPrefix+stripHeredoc(`
		ALTER TABLE "public"."users" ALTER COLUMN "name" DROP NOT NULL;
		`,
	))
	assertApplyOutput(t, createTable, nothingModified)
}

func TestPsqldefCitextExtension(t *testing.T) {
	resetTestDatabase()
	mustExecute("psql", "-Upostgres", "psqldef_test", "-c", "CREATE EXTENSION citext;")

	createTable := stripHeredoc(`
		CREATE TABLE users (
		  name citext
		);
		`,
	)
	assertApplyOutput(t, createTable, applyPrefix+createTable)
	assertApplyOutput(t, createTable, nothingModified)

	mustExecute("psql", "-Upostgres", "psqldef_test", "-c", "DROP TABLE users;")
	mustExecute("psql", "-Upostgres", "psqldef_test", "-c", "DROP EXTENSION citext;")
}

func TestPsqldefIgnoreExtension(t *testing.T) {
	resetTestDatabase()
	mustExecute("psql", "-Upostgres", "psqldef_test", "-c", "CREATE EXTENSION pg_buffercache;")

	createTable := stripHeredoc(`
		CREATE TABLE users (
		  id bigint NOT NULL,
		  name text,
		  age integer
		);
		`,
	)

	assertApplyOutput(t, createTable, applyPrefix+createTable)
	assertApplyOutput(t, createTable, nothingModified)

	mustExecute("psql", "-Upostgres", "psqldef_test", "-c", "DROP EXTENSION pg_buffercache;")
}

func TestPsqldefCreateTablePrimaryKey(t *testing.T) {
	resetTestDatabase()

	createTable := stripHeredoc(`
		CREATE TABLE users (
		  id bigint NOT NULL PRIMARY KEY,
		  name text
		);
		`,
	)
	assertApplyOutput(t, createTable, applyPrefix+createTable)
	assertApplyOutput(t, createTable, nothingModified)

	createTable = stripHeredoc(`
		CREATE TABLE users (
		  name text
		);`,
	)
	assertApplyOutput(t, createTable, applyPrefix+
		`ALTER TABLE "public"."users" DROP CONSTRAINT "users_pkey";`+"\n"+
		`ALTER TABLE "public"."users" DROP COLUMN "id";`+"\n",
	)
	assertApplyOutput(t, createTable, nothingModified)

	createTable = stripHeredoc(`
		CREATE TABLE users (
		  id bigint NOT NULL PRIMARY KEY,
		  name text
		);`,
	)
	assertApplyOutput(t, createTable, applyPrefix+stripHeredoc(`
		ALTER TABLE "public"."users" ADD COLUMN "id" bigint NOT NULL;
		ALTER TABLE "public"."users" ADD primary key ("id");
		`,
	))
	assertApplyOutput(t, createTable, nothingModified)
}

func TestPsqldefCreateTableConstraintPrimaryKey(t *testing.T) {
	resetTestDatabase()

	createTable := stripHeredoc(`
		CREATE TABLE users (
		  a integer,
		  b integer,
		  CONSTRAINT a_b_pkey PRIMARY KEY (a, b)
		);
		`,
	)
	assertApplyOutput(t, createTable, applyPrefix+createTable)
	assertApplyOutput(t, createTable, nothingModified)
}

func TestPsqldefCreateTableForeignKey(t *testing.T) {
	resetTestDatabase()

	createUsers := "CREATE TABLE users (id BIGINT PRIMARY KEY);\n"
	createPosts := stripHeredoc(`
		CREATE TABLE posts (
		  content text,
		  user_id bigint
		);
		`,
	)
	assertApplyOutput(t, createUsers+createPosts, applyPrefix+createUsers+createPosts)
	assertApplyOutput(t, createUsers+createPosts, nothingModified)

	createPosts = stripHeredoc(`
		CREATE TABLE posts (
		  content text,
		  user_id bigint,
		  CONSTRAINT posts_ibfk_1 FOREIGN KEY (user_id) REFERENCES users (id)
		);
		`,
	)
	assertApplyOutput(t, createUsers+createPosts, applyPrefix+
		`ALTER TABLE "public"."posts" ADD CONSTRAINT "posts_ibfk_1" FOREIGN KEY ("user_id") REFERENCES "users" ("id");`+"\n",
	)
	assertApplyOutput(t, createUsers+createPosts, nothingModified)

	createPosts = stripHeredoc(`
		CREATE TABLE posts (
		  content text,
		  user_id bigint,
		  CONSTRAINT posts_ibfk_1 FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE SET NULL ON UPDATE CASCADE
		);
		`,
	)
	assertApplyOutput(t, createUsers+createPosts, applyPrefix+
		`ALTER TABLE "public"."posts" DROP CONSTRAINT "posts_ibfk_1";`+"\n"+
		`ALTER TABLE "public"."posts" ADD CONSTRAINT "posts_ibfk_1" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON DELETE SET NULL ON UPDATE CASCADE;`+"\n",
	)
	assertApplyOutput(t, createUsers+createPosts, nothingModified)

	createPosts = stripHeredoc(`
		CREATE TABLE posts (
		  content text,
		  user_id bigint
		);
		`,
	)
	assertApplyOutput(t, createUsers+createPosts, applyPrefix+`ALTER TABLE "public"."posts" DROP CONSTRAINT "posts_ibfk_1";`+"\n")
	assertApplyOutput(t, createUsers+createPosts, nothingModified)
}

func TestPsqldefAddForeignKey(t *testing.T) {
	resetTestDatabase()

	createUsers := "CREATE TABLE users (id BIGINT PRIMARY KEY);\n"
	createPosts := stripHeredoc(`
		CREATE TABLE posts (
		  content text,
		  user_id bigint,
		  CONSTRAINT posts_ibfk_1 FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE SET NULL ON UPDATE CASCADE
		);
		`,
	)
	assertApplyOutput(t, createUsers+createPosts, applyPrefix+createUsers+createPosts)
	assertApplyOutput(t, createUsers+createPosts, nothingModified)

	createPosts = stripHeredoc(`
		CREATE TABLE posts (
		  content text,
		  user_id bigint
		);
		`,
	)
	addForeignKey := "ALTER TABLE ONLY public.posts ADD CONSTRAINT posts_ibfk_1 FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE SET NULL ON UPDATE CASCADE;\n"
	assertApplyOutput(t, createUsers+createPosts+addForeignKey, nothingModified)

}

func TestPsqldefCreateTableWithReferences(t *testing.T) {
	resetTestDatabase()

	createTableA := stripHeredoc(`
		CREATE TABLE a (
		  a_id INTEGER PRIMARY KEY,
		  my_text TEXT UNIQUE NOT NULL
		);
		`,
	)
	createTableB := stripHeredoc(`
		CREATE TABLE b (
		  b_id INTEGER PRIMARY KEY,
		  a_id INTEGER REFERENCES a,
		  a_my_text TEXT NOT NULL REFERENCES a (my_text)
		);
		`,
	)
	assertApplyOutput(t, createTableA+createTableB, applyPrefix+createTableA+createTableB)
	assertApplyOutput(t, createTableA+createTableB, nothingModified)

	createTableB = stripHeredoc(`
		CREATE TABLE b (
		  b_id INTEGER PRIMARY KEY,
		  a_id INTEGER,
		  a_my_text TEXT NOT NULL
		);
		`,
	)
	assertApplyOutput(t, createTableA+createTableB, applyPrefix+
		`ALTER TABLE "public"."b" DROP CONSTRAINT "b_a_id_fkey";`+"\n"+
		`ALTER TABLE "public"."b" DROP CONSTRAINT "b_a_my_text_fkey";`+"\n")
	assertApplyOutput(t, createTableA+createTableB, nothingModified)
}

func TestPsqldefCreateTableWithCheck(t *testing.T) {
	resetTestDatabase()

	createTable := stripHeredoc(`
		CREATE TABLE a (
		  a_id INTEGER PRIMARY KEY CHECK (a_id > 0),
		  my_text TEXT UNIQUE NOT NULL
		);
		`,
	)
	assertApplyOutput(t, createTable, applyPrefix+createTable)
	assertApplyOutput(t, createTable, nothingModified)

	createTable = stripHeredoc(`
		CREATE TABLE a (
		  a_id INTEGER PRIMARY KEY CHECK (a_id > 1),
		  my_text TEXT UNIQUE NOT NULL
		);
		`,
	)
	assertApplyOutput(t, createTable, applyPrefix+
		`ALTER TABLE "public"."a" DROP CONSTRAINT a_a_id_check;`+"\n"+
		`ALTER TABLE "public"."a" ADD CONSTRAINT a_a_id_check CHECK (a_id > 1);`+"\n")
	assertApplyOutput(t, createTable, nothingModified)

	createTable = stripHeredoc(`
		CREATE TABLE a (
		  a_id INTEGER PRIMARY KEY,
		  my_text TEXT UNIQUE NOT NULL
		);
		`,
	)
	assertApplyOutput(t, createTable, applyPrefix+
		`ALTER TABLE "public"."a" DROP CONSTRAINT a_a_id_check;`+"\n")
	assertApplyOutput(t, createTable, nothingModified)

	createTable = stripHeredoc(`
		CREATE TABLE a (
		  a_id INTEGER PRIMARY KEY CHECK (a_id > 2) NO INHERIT,
		  my_text TEXT UNIQUE NOT NULL
		);
		`,
	)
	assertApplyOutput(t, createTable, applyPrefix+
		`ALTER TABLE "public"."a" ADD CONSTRAINT a_a_id_check CHECK (a_id > 2) NO INHERIT;`+"\n")
	assertApplyOutput(t, createTable, nothingModified)

	createTable = stripHeredoc(`
		CREATE TABLE a (
		  a_id INTEGER PRIMARY KEY CHECK (a_id > 3) NO INHERIT,
		  my_text TEXT UNIQUE NOT NULL
		);
		`,
	)
	assertApplyOutput(t, createTable, applyPrefix+
		`ALTER TABLE "public"."a" DROP CONSTRAINT a_a_id_check;`+"\n"+
		`ALTER TABLE "public"."a" ADD CONSTRAINT a_a_id_check CHECK (a_id > 3) NO INHERIT;`+"\n")
	assertApplyOutput(t, createTable, nothingModified)
}

func TestPsqlddefCreatePolicy(t *testing.T) {
	resetTestDatabase()

	createUsers := "CREATE TABLE users (id BIGINT PRIMARY KEY, name character varying(100));\n"

	assertApplyOutput(t, createUsers, applyPrefix+createUsers)
	assertApplyOutput(t, createUsers, nothingModified)

	createPolicy := stripHeredoc(`
		CREATE POLICY p_users ON users AS PERMISSIVE FOR ALL TO PUBLIC USING (id = (current_user)::integer) WITH CHECK ((current_user)::integer = 1);
		`,
	)
	assertApplyOutput(t, createUsers+createPolicy, applyPrefix+
		"CREATE POLICY p_users ON users AS PERMISSIVE FOR ALL TO PUBLIC USING (id = (current_user)::integer) WITH CHECK ((current_user)::integer = 1);\n",
	)
	assertApplyOutput(t, createUsers+createPolicy, nothingModified)

	createPolicy = stripHeredoc(`
		CREATE POLICY p_users ON users AS RESTRICTIVE FOR ALL TO postgres USING (id = (current_user)::integer);
		`,
	)
	assertApplyOutput(t, createUsers+createPolicy, applyPrefix+stripHeredoc(`
		DROP POLICY "p_users" ON "public"."users";
		CREATE POLICY p_users ON users AS RESTRICTIVE FOR ALL TO postgres USING (id = (current_user)::integer);
		`,
	))
	assertApplyOutput(t, createUsers+createPolicy, nothingModified)

	createPolicy = stripHeredoc(`
		CREATE POLICY p_users ON users AS RESTRICTIVE FOR ALL TO postgres USING (true);
		`,
	)
	assertApplyOutput(t, createUsers+createPolicy, applyPrefix+stripHeredoc(`
		DROP POLICY "p_users" ON "public"."users";
		CREATE POLICY p_users ON users AS RESTRICTIVE FOR ALL TO postgres USING (true);
		`,
	))
	assertApplyOutput(t, createUsers+createPolicy, nothingModified)

	assertApplyOutput(t, createUsers, applyPrefix+`DROP POLICY "p_users" ON "public"."users";`+"\n")
	assertApplyOutput(t, createUsers, nothingModified)
}

func TestPsqldefCreateView(t *testing.T) {
	resetTestDatabase()

	createUsers := "CREATE TABLE users (id BIGINT PRIMARY KEY, name character varying(100));\n"
	createPosts := "CREATE TABLE posts (id BIGINT PRIMARY KEY, name character varying(100), user_id BIGINT, is_deleted boolean);\n"

	assertApplyOutput(t, createUsers+createPosts, applyPrefix+createUsers+createPosts)
	assertApplyOutput(t, createUsers+createPosts, nothingModified)

	createView := stripHeredoc(`
		CREATE VIEW view_user_posts AS SELECT p.id FROM (posts as p JOIN users as u ON ((p.user_id = u.id)));
		`,
	)
	assertApplyOutput(t, createUsers+createPosts+createView, applyPrefix+
		"CREATE VIEW view_user_posts AS SELECT p.id FROM (posts as p JOIN users as u ON ((p.user_id = u.id)));\n",
	)
	assertApplyOutput(t, createUsers+createPosts+createView, nothingModified)

	createView = stripHeredoc(`
		CREATE VIEW view_user_posts AS SELECT p.id from (posts p INNER JOIN users u ON ((p.user_id = u.id))) WHERE (p.is_deleted = FALSE);
		`,
	)
	assertApplyOutput(t, createUsers+createPosts+createView, applyPrefix+stripHeredoc(`
		CREATE OR REPLACE VIEW "public"."view_user_posts" AS select p.id from (posts as p join users as u on ((p.user_id = u.id))) where (p.is_deleted = false);
		`,
	))
	assertApplyOutput(t, createUsers+createPosts+createView, nothingModified)

	assertApplyOutput(t, createUsers+createPosts, applyPrefix+`DROP VIEW "public"."view_user_posts";`+"\n")
	assertApplyOutput(t, createUsers+createPosts, nothingModified)
}

func TestPsqldefDropPrimaryKey(t *testing.T) {
	createTable := stripHeredoc(`
		CREATE TABLE users (
		  id bigint NOT NULL PRIMARY KEY,
		  name text
		);`,
	)
	assertApply(t, createTable)

	createTable = stripHeredoc(`
		CREATE TABLE users (
		  id bigint NOT NULL,
		  name text
		);`,
	)
	assertApplyOutput(t, createTable, applyPrefix+`ALTER TABLE "public"."users" DROP CONSTRAINT "users_pkey";`+"\n")
	assertApplyOutput(t, createTable, nothingModified)
}

func TestPsqldefAddColumn(t *testing.T) {
	resetTestDatabase()

	createTable := stripHeredoc(`
		CREATE TABLE users (
		  id bigint NOT NULL,
		  name text
		);
		`,
	)
	assertApplyOutput(t, createTable, applyPrefix+createTable)
	assertApplyOutput(t, createTable, nothingModified)

	createTable = stripHeredoc(`
		CREATE TABLE users (
		  id bigint NOT NULL,
		  name text,
		  age integer
		);`,
	)
	assertApplyOutput(t, createTable, applyPrefix+`ALTER TABLE "public"."users" ADD COLUMN "age" integer;`+"\n")
	assertApplyOutput(t, createTable, nothingModified)

	createTable = stripHeredoc(`
		CREATE TABLE users (
		  id bigint NOT NULL,
		  age integer
		);`,
	)
	assertApplyOutput(t, createTable, applyPrefix+`ALTER TABLE "public"."users" DROP COLUMN "name";`+"\n")
	assertApplyOutput(t, createTable, nothingModified)
}

func TestPsqldefAddArrayColumn(t *testing.T) {
	resetTestDatabase()

	createTable := stripHeredoc(`
		CREATE TABLE users (
		  id integer
		);
		`,
	)
	assertApplyOutput(t, createTable, applyPrefix+createTable)
	assertApplyOutput(t, createTable, nothingModified)

	createTable = stripHeredoc(`
		CREATE TABLE users (
		  id integer,
		  name integer[]
		);`,
	)
	assertApplyOutput(t, createTable, applyPrefix+`ALTER TABLE "public"."users" ADD COLUMN "name" integer[];`+"\n")
	assertApplyOutput(t, createTable, nothingModified)
}

func TestPsqldefCreateIndex(t *testing.T) {
	resetTestDatabase()

	createTable := stripHeredoc(`
		CREATE TABLE users (
		  id bigint NOT NULL,
		  name text,
		  age integer
		);
		`,
	)
	createIndex1 := `CREATE INDEX "index_name" on users (name);` + "\n"
	createIndex2 := `CREATE UNIQUE INDEX "index_age" on users (age);` + "\n"
	assertApplyOutput(t, createTable+createIndex1+createIndex2, applyPrefix+createTable+createIndex1+createIndex2)
	assertApplyOutput(t, createTable+createIndex1+createIndex2, nothingModified)

	createIndex1 = `CREATE INDEX "index_name" on users (name, id);` + "\n"
	assertApplyOutput(t, createTable+createIndex1+createIndex2, applyPrefix+`DROP INDEX "index_name";`+"\n"+createIndex1)
	assertApplyOutput(t, createTable+createIndex1+createIndex2, nothingModified)

	createIndex1 = `CREATE UNIQUE INDEX "index_name" on users (name) WHERE age > 20;` + "\n"
	assertApplyOutput(t, createTable+createIndex1+createIndex2, applyPrefix+`DROP INDEX "index_name";`+"\n"+createIndex1)
	assertApplyOutput(t, createTable+createIndex1+createIndex2, nothingModified)

	assertApplyOutput(t, createTable, applyPrefix+`DROP INDEX "index_age";`+"\n"+`DROP INDEX "index_name";`+"\n")
	assertApplyOutput(t, createTable, nothingModified)
}

func TestPsqldefAddConstraintUnique(t *testing.T) {
	resetTestDatabase()

	createTable := stripHeredoc(`
		create table dummy(
		  column_a int not null,
		  column_b int not null,
		  column_c int not null
		);
		alter table dummy add constraint dummy_uniq unique (column_a, column_b);
		`,
	)
	assertApplyOutput(t, createTable, applyPrefix+createTable)
	assertApplyOutput(t, createTable, nothingModified)
}

func TestPsqldefCreateIndexWithKey(t *testing.T) {
	resetTestDatabase()

	createTable := stripHeredoc(`
		CREATE TABLE users (
		  "key" text
		);
		`,
	)
	createIndex := `CREATE INDEX "index_name" on users (key);`
	assertApplyOutput(t, createTable+createIndex, applyPrefix+createTable+createIndex+"\n")
	assertApplyOutput(t, createTable+createIndex, nothingModified)
}

func TestPsqldefCreateType(t *testing.T) {
	resetTestDatabase()

	createTable := stripHeredoc(`
		CREATE TYPE country AS ENUM ('us', 'jp');
		CREATE TABLE users (
		  id SERIAL PRIMARY KEY,
		  country country NOT NULL DEFAULT 'jp'::country
		);
		`,
	)
	assertApplyOutput(t, createTable, applyPrefix+createTable)
	assertApplyOutput(t, createTable, nothingModified)
}

func TestPsqldefColumnLiteral(t *testing.T) {
	resetTestDatabase()

	createTable := stripHeredoc(`
		CREATE TABLE users (
		  "id" bigint NOT NULL,
		  "name" text,
		  "age" integer
		);
		`,
	)
	assertApplyOutput(t, createTable, applyPrefix+createTable)
	assertApplyOutput(t, createTable, nothingModified)
}

func TestPsqldefDataTypes(t *testing.T) {
	resetTestDatabase()

	// TODO:
	//   - int8
	//   - serial8
	//   - box
	//   - bytea
	//   - cidr
	//   - circle
	//   - float8
	//   - inet
	//   - int4
	//   - interval [ fields ] [ (p) ]
	//   - line
	//   - lseg
	//   - macaddr
	//   - money
	//   - numeric [ (p, s) ]
	//   - decimal [ (p, s) ]
	//   - path
	//   - pg_lsn
	//   - point
	//   - polygon
	//   - real
	//   - float4
	//   - smallint
	//   - int2
	//   - smallserial
	//   - serial2
	//   - serial4
	//   - timetz
	//   - timestamptz
	//   - tsquer
	//   - tsvector
	//   - txid_snapshot
	//   - xml
	//
	// Remaining SQL spec: bit varying, interval, numeric, decimal, real,
	//   smallint, smallserial, xml
	createTable := stripHeredoc(`
		CREATE TABLE users (
		  c_array integer array,
		  c_array_bracket integer[],
		  c_bigint bigint,
		  c_bigserial bigserial,
		  c_bit bit,
		  c_bit_2 bit(2),
		  c_bool bool,
		  c_boolean boolean,
		  c_char_10 char(10),
		  c_character_20 character(20),
		  c_character_varying_30 character varying(30),
		  c_date date,
		  c_double_precision double precision,
		  c_json json,
		  c_jsonb jsonb,
		  c_timestamp timestamp,
		  c_timestamp_6 timestamp(6),
		  c_timestamp_tz timestamp with time zone,
		  c_timestamp_tz_6 timestamp(6) with time zone,
		  c_timestamp_tz_6_notnull timestamp(6) with time zone not null,
		  c_time time,
		  c_time_6 time(6),
		  c_time_tz time with time zone,
		  c_time_tz_6 time(6) with time zone,
		  c_time_tz_6_notnull time(6) with time zone not null,
		  c_int int,
		  c_integer integer,
		  c_serial serial,
		  c_text text,
		  c_uuid uuid,
		  c_varchar_40 varchar(40)
		);
		`,
	)

	assertApplyOutput(t, createTable, applyPrefix+createTable)
	assertApplyOutput(t, createTable, nothingModified) // Label for column type may change. Type will be examined.
}

//
// ----------------------- following tests are for CLI -----------------------
//

func TestPsqldefDryRun(t *testing.T) {
	resetTestDatabase()
	writeFile("schema.sql", stripHeredoc(`
	    CREATE TABLE users (
	        id bigint NOT NULL PRIMARY KEY,
	        age int
	    );`,
	))

	dryRun := assertedExecute(t, "./psqldef", "-Upostgres", "psqldef_test", "--dry-run", "--file", "schema.sql")
	apply := assertedExecute(t, "./psqldef", "-Upostgres", "psqldef_test", "--file", "schema.sql")
	assertEquals(t, dryRun, strings.Replace(apply, "Apply", "dry run", 1))
}

func TestPsqldefSkipDrop(t *testing.T) {
	resetTestDatabase()
	mustExecute("psql", "-Upostgres", "psqldef_test", "-c", stripHeredoc(`
		CREATE TABLE users (
		    id bigint NOT NULL PRIMARY KEY,
		    age int,
		    c_char_1 char,
		    c_char_10 char(10),
		    c_varchar_10 varchar(10),
		    c_varchar_unlimited varchar
		);`,
	))

	writeFile("schema.sql", "")

	skipDrop := assertedExecute(t, "./psqldef", "-Upostgres", "psqldef_test", "--skip-drop", "--file", "schema.sql")
	apply := assertedExecute(t, "./psqldef", "-Upostgres", "psqldef_test", "--file", "schema.sql")
	assertEquals(t, skipDrop, strings.Replace(apply, "DROP", "-- Skipped: DROP", 1))
}

func TestPsqldefExport(t *testing.T) {
	resetTestDatabase()
	out := assertedExecute(t, "./psqldef", "-Upostgres", "psqldef_test", "--export")
	assertEquals(t, out, "-- No table exists --\n")

	mustExecute("psql", "-Upostgres", "psqldef_test", "-c", stripHeredoc(`
		CREATE TABLE users (
		    id bigint NOT NULL PRIMARY KEY,
		    age int,
		    c_char_1 char unique,
		    c_char_10 char(10),
		    c_varchar_10 varchar(10),
		    c_varchar_unlimited varchar
		);`,
	))
	out = assertedExecute(t, "./psqldef", "-Upostgres", "psqldef_test", "--export")
	// workaround: local has `public.` but travis doesn't.
	assertEquals(t, strings.Replace(out, "public.users", "users", 2), stripHeredoc(`
		CREATE TABLE users (
		    "id" bigint NOT NULL,
		    "age" integer,
		    "c_char_1" character(1),
		    "c_char_10" character(10),
		    "c_varchar_10" character varying(10),
		    "c_varchar_unlimited" character varying,
		    PRIMARY KEY ("id")
		);
		CREATE UNIQUE INDEX users_c_char_1_key ON users USING btree (c_char_1);
		`,
	))
}

func TestPsqldefExportCompositePrimaryKey(t *testing.T) {
	resetTestDatabase()
	out := assertedExecute(t, "./psqldef", "-Upostgres", "psqldef_test", "--export")
	assertEquals(t, out, "-- No table exists --\n")

	mustExecute("psql", "-Upostgres", "psqldef_test", "-c", stripHeredoc(`
		CREATE TABLE users (
		    col1 character varying(40) NOT NULL,
		    col2 character varying(6) NOT NULL,
		    created_at timestamp NOT NULL,
		    PRIMARY KEY (col1, col2)
		);`,
	))
	out = assertedExecute(t, "./psqldef", "-Upostgres", "psqldef_test", "--export")
	// workaround: local has `public.` but travis doesn't.
	assertEquals(t, strings.Replace(out, "public.users", "users", 2), stripHeredoc(`
		CREATE TABLE users (
		    "col1" character varying(40) NOT NULL,
		    "col2" character varying(6) NOT NULL,
		    "created_at" timestamp NOT NULL,
		    PRIMARY KEY ("col1", "col2")
		);
		`,
	))
}

func TestPsqldefCreateTableWithIdentityColumn(t *testing.T) {
	resetTestDatabase()

	createTable := stripHeredoc(`
		CREATE TABLE color (
		  color_id INT GENERATED ALWAYS AS IDENTITY,
		  color_name VARCHAR NOT NULL
		);
		`,
	)
	assertApplyOutput(t, createTable, applyPrefix+createTable)
	assertApplyOutput(t, createTable, nothingModified)
}

func TestPsqldefAddingIdentityColumn(t *testing.T) {
	resetTestDatabase()

	createTable := stripHeredoc(`
		CREATE TABLE color (
		  color_id INT NOT NULL,
		  color_name VARCHAR NOT NULL
		);
		`,
	)
	assertApplyOutput(t, createTable, applyPrefix+createTable)
	assertApplyOutput(t, createTable, nothingModified)

	createTable = stripHeredoc(`
		CREATE TABLE color (
		  color_id INT GENERATED BY DEFAULT AS IDENTITY,
		  color_name VARCHAR NOT NULL
		);
		`,
	)
	assertApplyOutput(t, createTable, applyPrefix+`ALTER TABLE "public"."color" ALTER COLUMN "color_id" ADD GENERATED BY DEFAULT AS IDENTITY;`+"\n")
	assertApplyOutput(t, createTable, nothingModified)
}

func TestPsqldefRemovingIdentityColumn(t *testing.T) {
	resetTestDatabase()

	createTable := stripHeredoc(`
		CREATE TABLE color (
		  color_id INT GENERATED BY DEFAULT AS IDENTITY,
		  color_name VARCHAR NOT NULL
		);
		`,
	)
	assertApplyOutput(t, createTable, applyPrefix+createTable)
	assertApplyOutput(t, createTable, nothingModified)

	createTable = stripHeredoc(`
		CREATE TABLE color (
		  color_id INT NOT NULL,
		  color_name VARCHAR NOT NULL
		);
		`,
	)
	assertApplyOutput(t, createTable, applyPrefix+`ALTER TABLE "public"."color" ALTER COLUMN "color_id" DROP IDENTITY IF EXISTS;`+"\n")
	assertApplyOutput(t, createTable, nothingModified)
}

func TestPsqldefChangingIdentityColumn(t *testing.T) {
	resetTestDatabase()

	createTable := stripHeredoc(`
		CREATE TABLE color (
		  color_id INT GENERATED BY DEFAULT AS IDENTITY,
		  color_name VARCHAR NOT NULL
		);
		`,
	)
	assertApplyOutput(t, createTable, applyPrefix+createTable)
	assertApplyOutput(t, createTable, nothingModified)

	createTable = stripHeredoc(`
		CREATE TABLE color (
		  color_id INT GENERATED ALWAYS AS IDENTITY,
		  color_name VARCHAR NOT NULL
		);
		`,
	)
	assertApplyOutput(t, createTable, applyPrefix+`ALTER TABLE "public"."color" ALTER COLUMN "color_id" SET GENERATED ALWAYS;`+"\n")
	assertApplyOutput(t, createTable, nothingModified)
}

func TestPsqldefCreateIdentityColumnWithSequenceOption(t *testing.T) {
	resetTestDatabase()

	createTableWithSequence1 := stripHeredoc(`
		CREATE TABLE voltages (
		  volt int GENERATED BY DEFAULT AS IDENTITY
		    (START WITH -200 INCREMENT BY 10 MINVALUE -200 MAXVALUE 200)
		);
		`,
	)
	createTableWithoutSequence := stripHeredoc(`
		CREATE TABLE voltages (
		  volt int GENERATED BY DEFAULT AS IDENTITY
		);
		`,
	)

	assertApplyOutput(t, createTableWithSequence1, applyPrefix+createTableWithSequence1)
	assertApplyOutput(t, createTableWithoutSequence, nothingModified)

	createTableWithSequence2 := stripHeredoc(`
		CREATE TABLE voltages (
		  volt int GENERATED BY DEFAULT AS IDENTITY
		    (START WITH -100 INCREMENT BY 5 MINVALUE -100 MAXVALUE 100)
		);
		`,
	)

	// not support changing sequence option
	assertApplyOutput(t, createTableWithSequence2, nothingModified)
}

func TestPsqldefModifyIdentityColumnWithSequenceOption(t *testing.T) {
	resetTestDatabase()

	createTable := stripHeredoc(`
		CREATE TABLE voltages (
		  volt int
		);
		`,
	)
	assertApplyOutput(t, createTable, applyPrefix+createTable)

	createTableWithSequence := stripHeredoc(`
		CREATE TABLE voltages (
		  volt int GENERATED BY DEFAULT AS IDENTITY
		    (START WITH -100 INCREMENT BY 5 MINVALUE -100 MAXVALUE 100)
		);
		`,
	)
	alter1 := `ALTER TABLE "public"."voltages" ALTER COLUMN "volt" SET NOT NULL;`
	alter2 := `ALTER TABLE "public"."voltages" ALTER COLUMN "volt" ADD GENERATED BY DEFAULT AS IDENTITY (START WITH -100 INCREMENT BY 5 MINVALUE -100 MAXVALUE 100);`
	assertApplyOutput(t, createTableWithSequence, applyPrefix+alter1+"\n"+alter2+"\n")

	createTableWithoutSequence := stripHeredoc(`
		CREATE TABLE voltages (
		  volt int GENERATED BY DEFAULT AS IDENTITY
		);
		`,
	)

	// not support changing sequence option
	assertApplyOutput(t, createTableWithoutSequence, nothingModified)
}

func TestPsqldefAddIdentityColumnWithSequenceOption(t *testing.T) {
	resetTestDatabase()

	createTable := stripHeredoc(`
		CREATE TABLE voltages (
		  name varchar NOT NULL
		);
		`,
	)
	assertApplyOutput(t, createTable, applyPrefix+createTable)

	createTableWithSequence := stripHeredoc(`
		CREATE TABLE voltages (
		  name varchar NOT NULL,
		  volt int GENERATED BY DEFAULT AS IDENTITY
		    (START WITH -100 INCREMENT BY 5 MINVALUE -100 MAXVALUE 100)
		);
		`,
	)
	alter := `ALTER TABLE "public"."voltages" ADD COLUMN "volt" int GENERATED BY DEFAULT AS IDENTITY (START WITH -100 INCREMENT BY 5 MINVALUE -100 MAXVALUE 100);`
	assertApplyOutput(t, createTableWithSequence, applyPrefix+alter+"\n")

	createTableWithoutSequence := stripHeredoc(`
		CREATE TABLE voltages (
		  name varchar NOT NULL,
		  volt int GENERATED BY DEFAULT AS IDENTITY
		);
		`,
	)

	// not support changing sequence option
	assertApplyOutput(t, createTableWithoutSequence, nothingModified)
}

func TestPsqldefAlterTableUniqueConstraint(t *testing.T) {
	resetTestDatabase()

	createPosts := stripHeredoc(`
		CREATE TABLE posts (
		  content text,
		  slug varchar(100)
		);
		`,
	)
	assertApplyOutput(t, createPosts, applyPrefix+createPosts)
	assertApplyOutput(t, createPosts, nothingModified)

	alterTable := "ALTER TABLE posts ADD CONSTRAINT posts_slug_unique UNIQUE (slug);"
	assertApplyOutput(t, createPosts+alterTable, applyPrefix+
		`ALTER TABLE "public"."posts" ADD CONSTRAINT "posts_slug_unique" UNIQUE ("slug");`+"\n",
	)
	assertApplyOutput(t, createPosts+alterTable, nothingModified)

	alterTable = "ALTER TABLE posts ADD CONSTRAINT posts_slug_unique UNIQUE (slug) NOT DEFERRABLE INITIALLY IMMEDIATE;"
	assertApplyOutput(t, createPosts+alterTable, nothingModified)

	alterTable = "ALTER TABLE posts ADD CONSTRAINT posts_slug_unique UNIQUE (slug) DEFERRABLE INITIALLY IMMEDIATE;"
	assertApplyOutput(t, createPosts+alterTable, applyPrefix+
		`ALTER TABLE "public"."posts" DROP CONSTRAINT "posts_slug_unique";`+"\n"+
		`ALTER TABLE "public"."posts" ADD CONSTRAINT "posts_slug_unique" UNIQUE ("slug") DEFERRABLE INITIALLY IMMEDIATE;`+"\n",
	)
	assertApplyOutput(t, createPosts+alterTable, nothingModified)

	alterTable = "ALTER TABLE posts ADD CONSTRAINT posts_slug_unique UNIQUE (slug) DEFERRABLE INITIALLY DEFERRED;"
	assertApplyOutput(t, createPosts+alterTable, applyPrefix+
		`ALTER TABLE "public"."posts" DROP CONSTRAINT "posts_slug_unique";`+"\n"+
		`ALTER TABLE "public"."posts" ADD CONSTRAINT "posts_slug_unique" UNIQUE ("slug") DEFERRABLE INITIALLY DEFERRED;`+"\n",
	)
	assertApplyOutput(t, createPosts+alterTable, nothingModified)

	assertApplyOutput(t, createPosts, applyPrefix+`ALTER TABLE "public"."posts" DROP CONSTRAINT "posts_slug_unique";`+"\n")
	assertApplyOutput(t, createPosts, nothingModified)
}

func TestPsqldefHelp(t *testing.T) {
	_, err := execute("./psqldef", "--help")
	if err != nil {
		t.Errorf("failed to run --help: %s", err)
	}

	out, err := execute("./psqldef")
	if err == nil {
		t.Errorf("no database must be error, but successfully got: %s", out)
	}
}

func TestMain(m *testing.M) {
	resetTestDatabase()
	mustExecute("go", "build")
	status := m.Run()
	_ = os.Remove("psqldef")
	_ = os.Remove("schema.sql")
	os.Exit(status)
}

func assertApply(t *testing.T, schema string) {
	t.Helper()
	writeFile("schema.sql", schema)
	assertedExecute(t, "./psqldef", "-Upostgres", "psqldef_test", "--file", "schema.sql")
}

func assertApplyOutput(t *testing.T, schema string, expected string) {
	t.Helper()
	writeFile("schema.sql", schema)
	actual := assertedExecute(t, "./psqldef", "-Upostgres", "psqldef_test", "--file", "schema.sql")
	assertEquals(t, actual, expected)
}

func mustExecute(command string, args ...string) {
	out, err := execute(command, args...)
	if err != nil {
		log.Printf("failed to execute '%s %s': `%s`", command, strings.Join(args, " "), out)
		log.Fatal(err)
	}
}

func assertedExecute(t *testing.T, command string, args ...string) string {
	t.Helper()
	out, err := execute(command, args...)
	if err != nil {
		t.Errorf("failed to execute '%s %s' (error: '%s'): `%s`", command, strings.Join(args, " "), err, out)
	}
	return out
}

func assertEquals(t *testing.T, actual string, expected string) {
	t.Helper()
	if expected != actual {
		t.Errorf("expected '%s' but got '%s'", expected, actual)
	}
}

func execute(command string, args ...string) (string, error) {
	cmd := exec.Command(command, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func resetTestDatabase() {
	mustExecute("psql", "-Upostgres", "-c", "DROP DATABASE IF EXISTS psqldef_test;")
	mustExecute("psql", "-Upostgres", "-c", "CREATE DATABASE psqldef_test;")
}

func writeFile(path string, content string) {
	file, err := os.Create(path)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	file.Write(([]byte)(content))
}

func stripHeredoc(heredoc string) string {
	heredoc = strings.TrimPrefix(heredoc, "\n")
	re := regexp.MustCompilePOSIX("^\t*")
	return re.ReplaceAllLiteralString(heredoc, "")
}
