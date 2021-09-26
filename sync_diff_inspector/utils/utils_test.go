// Copyright 2021 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package utils

import (
	"context"
	"database/sql/driver"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	. "github.com/pingcap/check"
	"github.com/pingcap/parser"
	"github.com/pingcap/tidb-tools/pkg/dbutil"
	"github.com/pingcap/tidb-tools/sync_diff_inspector/chunk"
)

func TestClient(t *testing.T) {
	TestingT(t)
}

var _ = Suite(&testUtilsSuite{})

type testUtilsSuite struct{}

type tableCaseType struct {
	schema         string
	table          string
	createTableSQL string
	rowColumns     []string
	rows           [][]driver.Value
	indices        []string
	sels           []float64
	selected       string
}

func (*testUtilsSuite) TestWorkerPool(c *C) {
	pool := NewWorkerPool(2, "test")
	infoCh := make(chan uint64)
	doneCh := make(chan struct{})
	var v uint64 = 0
	pool.Apply(func() {
		infoCh <- 2
	})
	pool.Apply(func() {
		new_v := <-infoCh
		v = new_v
		doneCh <- struct{}{}
	})
	<-doneCh
	c.Assert(v, Equals, uint64(2))
	c.Assert(pool.HasWorker(), IsTrue)
	pool.WaitFinished()
}

func (*testUtilsSuite) TestStringsToInterface(c *C) {
	res := []interface{}{"1", "2", "3"}
	c.Assert(res[0], Equals, "1")
	c.Assert(res[1], Equals, "2")
	c.Assert(res[2], Equals, "3")

	c.Assert(MinLenInSlices([][]string{{"123", "324", "r32"}, {"32", "23"}}), Equals, 2)

	expectSlice := []string{"2", "3", "4"}
	sliceMap := SliceToMap(expectSlice)
	for _, expect := range expectSlice {
		_, ok := sliceMap[expect]
		c.Assert(ok, IsTrue)
	}
	c.Assert(len(sliceMap), Equals, len(expectSlice))

	c.Assert(UniqueID("123", "456"), Equals, "123:456")

}

func (*testUtilsSuite) TestBasicTableUtilOperation(c *C) {
	createTableSQL := "create table `test`.`test`(`a` int, `b` varchar(10), `c` float, `d` datetime, primary key(`a`, `b`))"
	tableInfo, err := dbutil.GetTableInfoBySQL(createTableSQL, parser.New())
	c.Assert(err, IsNil)

	query, orderKeyCols := GetTableRowsQueryFormat("test", "test", tableInfo, "123")
	c.Assert(query, Equals, "SELECT /*!40001 SQL_NO_CACHE */ `a`, `b`, `c`, `d` FROM `test`.`test` WHERE %s ORDER BY `a`,`b` COLLATE \"123\"")
	expectName := []string{"a", "b"}
	for i, col := range orderKeyCols {
		c.Assert(col.Name.O, Equals, expectName[i])
	}

	data1 := map[string]*dbutil.ColumnData{
		"a": {Data: []byte("1"), IsNull: false},
		"b": {Data: []byte("a"), IsNull: false},
		"c": {Data: []byte("1.22"), IsNull: false},
		"d": {Data: []byte("sdf"), IsNull: false},
	}
	data2 := map[string]*dbutil.ColumnData{
		"a": {Data: []byte("1"), IsNull: false},
		"b": {Data: []byte("b"), IsNull: false},
		"c": {Data: []byte("2.22"), IsNull: false},
		"d": {Data: []byte("sdf"), IsNull: false},
	}
	data3 := map[string]*dbutil.ColumnData{
		"a": {Data: []byte("2"), IsNull: false},
		"b": {Data: []byte("a"), IsNull: false},
		"c": {Data: []byte("0.22"), IsNull: false},
		"d": {Data: []byte("asdf"), IsNull: false},
	}
	data4 := map[string]*dbutil.ColumnData{
		"a": {Data: []byte("1"), IsNull: false},
		"b": {Data: []byte("a"), IsNull: true},
		"c": {Data: []byte("0.221"), IsNull: false},
		"d": {Data: []byte("asdf"), IsNull: false},
	}
	data5 := map[string]*dbutil.ColumnData{
		"a": {Data: []byte("2"), IsNull: false},
		"b": {Data: []byte("a"), IsNull: true},
		"c": {Data: []byte("0.222"), IsNull: false},
		"d": {Data: []byte("asdf"), IsNull: false},
	}
	data6 := map[string]*dbutil.ColumnData{
		"a": {Data: []byte("1"), IsNull: true},
		"b": {Data: []byte("a"), IsNull: false},
		"c": {Data: []byte("0.2221"), IsNull: false},
		"d": {Data: []byte("asdf"), IsNull: false},
	}

	data7 := map[string]*dbutil.ColumnData{
		"a": {Data: []byte("1"), IsNull: true},
		"b": {Data: []byte("a"), IsNull: false},
		"c": {Data: []byte("0.2221"), IsNull: false},
		"d": {Data: []byte("asdf"), IsNull: false},
	}

	columns := tableInfo.Columns

	c.Assert(GenerateReplaceDML(data1, tableInfo, "schema"), Equals, "REPLACE INTO `schema`.`test`(`a`,`b`,`c`,`d`) VALUES (1,'a',1.22,'sdf');")
	c.Assert(GenerateReplaceDMLWithAnnotation(data1, data2, tableInfo, "schema"), Equals,
		"/*\n"+
			"  DIFF COLUMNS ╏ `B` ╏ `C`   \n"+
			"╍╍╍╍╍╍╍╍╍╍╍╍╍╍╍╋╍╍╍╍╍╋╍╍╍╍╍╍╍\n"+
			"  source data  ╏ 'a' ╏ 1.22  \n"+
			"╍╍╍╍╍╍╍╍╍╍╍╍╍╍╍╋╍╍╍╍╍╋╍╍╍╍╍╍╍\n"+
			"  target data  ╏ 'b' ╏ 2.22  \n"+
			"╍╍╍╍╍╍╍╍╍╍╍╍╍╍╍╋╍╍╍╍╍╋╍╍╍╍╍╍╍\n"+
			"*/\n"+
			"REPLACE INTO `schema`.`test`(`a`,`b`,`c`,`d`) VALUES (1,'a',1.22,'sdf');")
	c.Assert(GenerateDeleteDML(data1, tableInfo, "schema"), Equals, "DELETE FROM `schema`.`test` WHERE `a` = 1 AND `b` = 'a' AND `c` = 1.22 AND `d` = 'sdf';")

	// same
	equal, cmp, err := CompareData(data1, data1, orderKeyCols, columns)
	c.Assert(err, IsNil)
	c.Assert(cmp, Equals, int32(0))
	c.Assert(equal, IsTrue)

	// orderkey same but other column different
	equal, cmp, err = CompareData(data1, data3, orderKeyCols, columns)
	c.Assert(err, IsNil)
	c.Assert(cmp, Equals, int32(-1))
	c.Assert(equal, IsFalse)

	equal, cmp, err = CompareData(data3, data1, orderKeyCols, columns)
	c.Assert(err, IsNil)
	c.Assert(cmp, Equals, int32(1))
	c.Assert(equal, IsFalse)

	// orderKey different
	equal, cmp, err = CompareData(data1, data2, orderKeyCols, columns)
	c.Assert(err, IsNil)
	c.Assert(cmp, Equals, int32(-1))
	c.Assert(equal, IsFalse)

	equal, cmp, err = CompareData(data2, data1, orderKeyCols, columns)
	c.Assert(err, IsNil)
	c.Assert(cmp, Equals, int32(1))
	c.Assert(equal, IsFalse)

	equal, cmp, err = CompareData(data4, data1, orderKeyCols, columns)
	c.Assert(err, IsNil)
	c.Assert(cmp, Equals, int32(0))
	c.Assert(equal, IsFalse)

	equal, cmp, err = CompareData(data1, data4, orderKeyCols, columns)
	c.Assert(err, IsNil)
	c.Assert(cmp, Equals, int32(0))
	c.Assert(equal, IsFalse)

	equal, cmp, err = CompareData(data5, data4, orderKeyCols, columns)
	c.Assert(err, IsNil)
	c.Assert(cmp, Equals, int32(1))
	c.Assert(equal, IsFalse)

	equal, cmp, err = CompareData(data4, data5, orderKeyCols, columns)
	c.Assert(err, IsNil)
	c.Assert(cmp, Equals, int32(-1))
	c.Assert(equal, IsFalse)

	equal, cmp, err = CompareData(data4, data6, orderKeyCols, columns)
	c.Assert(err, IsNil)
	c.Assert(cmp, Equals, int32(1))
	c.Assert(equal, IsFalse)

	equal, cmp, err = CompareData(data6, data4, orderKeyCols, columns)
	c.Assert(err, IsNil)
	c.Assert(cmp, Equals, int32(-1))
	c.Assert(equal, IsFalse)

	equal, cmp, err = CompareData(data6, data7, orderKeyCols, columns)
	c.Assert(err, IsNil)
	c.Assert(cmp, Equals, int32(0))
	c.Assert(equal, IsTrue)

	// Test ignore columns
	createTableSQL = "create table `test`.`test`(`a` int, `c` float, `b` varchar(10), `d` datetime, primary key(`a`, `b`), key(`c`, `d`))"
	tableInfo, err = dbutil.GetTableInfoBySQL(createTableSQL, parser.New())
	c.Assert(err, IsNil)

	c.Assert(len(tableInfo.Indices), Equals, 2)
	c.Assert(len(tableInfo.Columns), Equals, 4)
	c.Assert(tableInfo.Indices[0].Columns[1].Name.O, Equals, "b")
	c.Assert(tableInfo.Indices[0].Columns[1].Offset, Equals, 2)
	info := IgnoreColumns(tableInfo, []string{"c"})
	c.Assert(len(info.Indices), Equals, 1)
	c.Assert(len(info.Columns), Equals, 3)
	c.Assert(tableInfo.Indices[0].Columns[1].Name.O, Equals, "b")
	c.Assert(tableInfo.Indices[0].Columns[1].Offset, Equals, 1)
}

func (*testUtilsSuite) TestGetCountAndCRC32Checksum(c *C) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	conn, mock, err := sqlmock.New()
	c.Assert(err, IsNil)
	defer conn.Close()

	createTableSQL := "create table `test`.`test`(`a` int, `c` float, `b` varchar(10), `d` datetime, primary key(`a`, `b`), key(`c`, `d`))"
	tableInfo, err := dbutil.GetTableInfoBySQL(createTableSQL, parser.New())
	c.Assert(err, IsNil)

	mock.ExpectQuery("SELECT COUNT.*FROM `test_schema`\\.`test_table` WHERE \\[23 45\\].*").WithArgs("123", "234").WillReturnRows(sqlmock.NewRows([]string{"CNT", "CHECKSUM"}).AddRow(123, 456))

	count, checksum, err := GetCountAndCRC32Checksum(ctx, conn, "test_schema", "test_table", tableInfo, "[23 45]", []interface{}{"123", "234"})
	c.Assert(err, IsNil)
	c.Assert(count, Equals, int64(123))
	c.Assert(checksum, Equals, int64(456))
}

func (*testUtilsSuite) TestGetApproximateMid(c *C) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	conn, mock, err := sqlmock.New()
	c.Assert(err, IsNil)
	defer conn.Close()

	createTableSQL := "create table `test`.`test`(`a` int, `b` varchar(10), primary key(`a`, `b`))"
	tableInfo, err := dbutil.GetTableInfoBySQL(createTableSQL, parser.New())
	c.Assert(err, IsNil)

	rows := sqlmock.NewRows([]string{"a", "b"}).AddRow("5", "10")
	mock.ExpectQuery("SELECT `a`, `b` FROM `test`\\.`test_utils` WHERE 2222.* LIMIT 10,1*").WithArgs("aaaa").WillReturnRows(rows)

	data, err := GetApproximateMidBySize(ctx, conn, "test", "test_utils", tableInfo, "2222", []interface{}{"aaaa"}, 20)
	c.Assert(err, IsNil)
	c.Assert(data["a"], Equals, "5")
	c.Assert(data["b"], Equals, "10")
}

func (*testUtilsSuite) TestGenerateSQLs(c *C) {
	createTableSQL := "CREATE TABLE `diff_test`.`atest` (`id` int(24), `name` varchar(24), `birthday` datetime, `update_time` time, `money` decimal(20,2), `id_gen` int(11) GENERATED ALWAYS AS ((`id` + 1)) VIRTUAL, primary key(`id`, `name`))"
	tableInfo, err := dbutil.GetTableInfoBySQL(createTableSQL, parser.New())
	c.Assert(err, IsNil)

	rowsData := map[string]*dbutil.ColumnData{
		"id":          {Data: []byte("1"), IsNull: false},
		"name":        {Data: []byte("xxx"), IsNull: false},
		"birthday":    {Data: []byte("2018-01-01 00:00:00"), IsNull: false},
		"update_time": {Data: []byte("10:10:10"), IsNull: false},
		"money":       {Data: []byte("11.1111"), IsNull: false},
		"id_gen":      {Data: []byte("2"), IsNull: false}, // generated column should not be contained in fix sql
	}

	replaceSQL := GenerateReplaceDML(rowsData, tableInfo, "diff_test")
	deleteSQL := GenerateDeleteDML(rowsData, tableInfo, "diff_test")
	c.Assert(replaceSQL, Equals, "REPLACE INTO `diff_test`.`atest`(`id`,`name`,`birthday`,`update_time`,`money`) VALUES (1,'xxx','2018-01-01 00:00:00','10:10:10',11.1111);")
	c.Assert(deleteSQL, Equals, "DELETE FROM `diff_test`.`atest` WHERE `id` = 1 AND `name` = 'xxx' AND `birthday` = '2018-01-01 00:00:00' AND `update_time` = '10:10:10' AND `money` = 11.1111;")

	// test the unique key
	createTableSQL2 := "CREATE TABLE `diff_test`.`atest` (`id` int(24), `name` varchar(24), `birthday` datetime, `update_time` time, `money` decimal(20,2), unique key(`id`, `name`))"
	tableInfo2, err := dbutil.GetTableInfoBySQL(createTableSQL2, parser.New())
	c.Assert(err, IsNil)
	replaceSQL = GenerateReplaceDML(rowsData, tableInfo2, "diff_test")
	deleteSQL = GenerateDeleteDML(rowsData, tableInfo2, "diff_test")
	c.Assert(replaceSQL, Equals, "REPLACE INTO `diff_test`.`atest`(`id`,`name`,`birthday`,`update_time`,`money`) VALUES (1,'xxx','2018-01-01 00:00:00','10:10:10',11.1111);")
	c.Assert(deleteSQL, Equals, "DELETE FROM `diff_test`.`atest` WHERE `id` = 1 AND `name` = 'xxx' AND `birthday` = '2018-01-01 00:00:00' AND `update_time` = '10:10:10' AND `money` = 11.1111;")

	// test value is nil
	rowsData["name"] = &dbutil.ColumnData{Data: []byte(""), IsNull: true}
	replaceSQL = GenerateReplaceDML(rowsData, tableInfo, "diff_test")
	deleteSQL = GenerateDeleteDML(rowsData, tableInfo, "diff_test")
	c.Assert(replaceSQL, Equals, "REPLACE INTO `diff_test`.`atest`(`id`,`name`,`birthday`,`update_time`,`money`) VALUES (1,NULL,'2018-01-01 00:00:00','10:10:10',11.1111);")
	c.Assert(deleteSQL, Equals, "DELETE FROM `diff_test`.`atest` WHERE `id` = 1 AND `name` is NULL AND `birthday` = '2018-01-01 00:00:00' AND `update_time` = '10:10:10' AND `money` = 11.1111;")

	rowsData["id"] = &dbutil.ColumnData{Data: []byte(""), IsNull: true}
	replaceSQL = GenerateReplaceDML(rowsData, tableInfo, "diff_test")
	deleteSQL = GenerateDeleteDML(rowsData, tableInfo, "diff_test")
	c.Assert(replaceSQL, Equals, "REPLACE INTO `diff_test`.`atest`(`id`,`name`,`birthday`,`update_time`,`money`) VALUES (NULL,NULL,'2018-01-01 00:00:00','10:10:10',11.1111);")
	c.Assert(deleteSQL, Equals, "DELETE FROM `diff_test`.`atest` WHERE `id` is NULL AND `name` is NULL AND `birthday` = '2018-01-01 00:00:00' AND `update_time` = '10:10:10' AND `money` = 11.1111;")

	// test value with "'"
	rowsData["name"] = &dbutil.ColumnData{Data: []byte("a'a"), IsNull: false}
	replaceSQL = GenerateReplaceDML(rowsData, tableInfo, "diff_test")
	deleteSQL = GenerateDeleteDML(rowsData, tableInfo, "diff_test")
	c.Assert(replaceSQL, Equals, "REPLACE INTO `diff_test`.`atest`(`id`,`name`,`birthday`,`update_time`,`money`) VALUES (NULL,'a\\'a','2018-01-01 00:00:00','10:10:10',11.1111);")
	c.Assert(deleteSQL, Equals, "DELETE FROM `diff_test`.`atest` WHERE `id` is NULL AND `name` = 'a\\'a' AND `birthday` = '2018-01-01 00:00:00' AND `update_time` = '10:10:10' AND `money` = 11.1111;")
}

func (s *testUtilsSuite) TestIgnoreColumns(c *C) {
	createTableSQL1 := "CREATE TABLE `test`.`atest` (`a` int, `b` int, `c` int, `d` int, primary key(`a`))"
	tableInfo1, err := dbutil.GetTableInfoBySQL(createTableSQL1, parser.New())
	c.Assert(err, IsNil)
	tbInfo := IgnoreColumns(tableInfo1, []string{"a"})
	c.Assert(tbInfo.Columns, HasLen, 3)
	c.Assert(tbInfo.Indices, HasLen, 0)
	c.Assert(tbInfo.Columns[2].Offset, Equals, 2)

	createTableSQL2 := "CREATE TABLE `test`.`atest` (`a` int, `b` int, `c` int, `d` int, primary key(`a`), index idx(`b`, `c`))"
	tableInfo2, err := dbutil.GetTableInfoBySQL(createTableSQL2, parser.New())
	c.Assert(err, IsNil)
	tbInfo = IgnoreColumns(tableInfo2, []string{"a", "b"})
	c.Assert(tbInfo.Columns, HasLen, 2)
	c.Assert(tbInfo.Indices, HasLen, 0)

	createTableSQL3 := "CREATE TABLE `test`.`atest` (`a` int, `b` int, `c` int, `d` int, primary key(`a`), index idx(`b`, `c`))"
	tableInfo3, err := dbutil.GetTableInfoBySQL(createTableSQL3, parser.New())
	c.Assert(err, IsNil)
	tbInfo = IgnoreColumns(tableInfo3, []string{"b", "c"})
	c.Assert(tbInfo.Columns, HasLen, 2)
	c.Assert(tbInfo.Indices, HasLen, 1)
}

func (s *testUtilsSuite) TestGetTableSize(c *C) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, mock, err := sqlmock.New()
	c.Assert(err, IsNil)
	defer conn.Close()
	dataRows := sqlmock.NewRows([]string{"a", "b"})
	rowNums := 1000
	for k := 0; k < rowNums; k++ {
		str := fmt.Sprintf("%d", k)
		dataRows.AddRow(str, str)
	}
	sizeRows := sqlmock.NewRows([]string{"data"})
	sizeRows.AddRow("8000")
	mock.ExpectQuery("data").WillReturnRows(sizeRows)
	size, err := GetTableSize(ctx, conn, "test", "test")
	c.Assert(err, IsNil)
	c.Assert(size, Equals, int64(8000))
}

func (s *testUtilsSuite) TestGetBetterIndex(c *C) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, mock, err := sqlmock.New()
	c.Assert(err, IsNil)
	defer conn.Close()
	tableCases := []*tableCaseType{
		{
			schema:         "single_index",
			table:          "test1",
			createTableSQL: "CREATE TABLE `single_index`.`test1` (`a` int, `b` char, primary key(`a`), index(`b`))",
			rowColumns:     []string{"a", "b"},
			rows: [][]driver.Value{
				{"1", "a"},
				{"2", "a"},
				{"3", "b"},
				{"4", "b"},
				{"5", "c"},
				{"6", "c"},
				{"7", "d"},
				{"8", "d"},
				{"9", "e"},
				{"A", "e"},
				{"B", "f"},
				{"C", "f"},
			},
			indices:  []string{"PRIMARY", "b"},
			sels:     []float64{1.0, 0.5},
			selected: "PRIMARY",
		}, {
			schema:         "single_index",
			table:          "test1",
			createTableSQL: "CREATE TABLE `single_index`.`test1` (`a` int, `b` char, index(a), index(b))",
			rowColumns:     []string{"a", "b"},
			rows: [][]driver.Value{
				{"1", "a"},
				{"2", "a"},
				{"3", "b"},
				{"4", "b"},
				{"5", "c"},
				{"6", "c"},
				{"7", "d"},
				{"8", "d"},
				{"9", "e"},
				{"A", "e"},
				{"B", "f"},
				{"C", "f"},
			},
			indices:  []string{"a", "b"},
			sels:     []float64{1.0, 0.5},
			selected: "a",
		},
	}
	tableCase := tableCases[0]
	tableInfo, err := dbutil.GetTableInfoBySQL(tableCase.createTableSQL, parser.New())
	c.Assert(err, IsNil)
	indices := dbutil.FindAllIndex(tableInfo)
	for i, index := range indices {
		c.Assert(index.Name.O, Equals, tableCase.indices[i])
	}
	for i, col := range tableCase.rowColumns {
		retRows := sqlmock.NewRows([]string{"SEL"})
		retRows.AddRow(tableCase.sels[i])
		mock.ExpectQuery("SELECT").WillReturnRows(retRows)
		sel, err := GetSelectivity(ctx, conn, tableCase.schema, tableCase.table, col, tableInfo)
		c.Assert(err, IsNil)
		c.Assert(sel, Equals, tableCase.sels[i])
	}
	indices, err = GetBetterIndex(ctx, conn, "single_index", "test1", tableInfo)
	c.Assert(err, IsNil)
	c.Assert(indices[0].Name.O, Equals, tableCase.selected)

	tableCase = tableCases[1]
	tableInfo, err = dbutil.GetTableInfoBySQL(tableCase.createTableSQL, parser.New())
	c.Assert(err, IsNil)
	indices = dbutil.FindAllIndex(tableInfo)
	for i, index := range indices {
		c.Assert(index.Name.O, Equals, tableCase.indices[i])
	}
	for i, col := range tableCase.rowColumns {
		retRows := sqlmock.NewRows([]string{"SEL"})
		retRows.AddRow(tableCase.sels[i])
		mock.ExpectQuery("SELECT").WillReturnRows(retRows)
		sel, err := GetSelectivity(ctx, conn, tableCase.schema, tableCase.table, col, tableInfo)
		c.Assert(err, IsNil)
		c.Assert(sel, Equals, tableCase.sels[i])
	}
	mock.ExpectQuery("SELECT COUNT\\(DISTINCT `a.*").WillReturnRows(sqlmock.NewRows([]string{"SEL"}).AddRow("2"))
	mock.ExpectQuery("SELECT COUNT\\(DISTINCT `b.*").WillReturnRows(sqlmock.NewRows([]string{"SEL"}).AddRow("5"))
	indices, err = GetBetterIndex(ctx, conn, "single_index", "test1", tableInfo)
	c.Assert(err, IsNil)
	c.Assert(indices[0].Name.O, Equals, tableCase.selected)

}

func (s *testUtilsSuite) TestCalculateChunkSize(c *C) {
	c.Assert(CalculateChunkSize(1000), Equals, int64(50000))
	c.Assert(CalculateChunkSize(1000000000), Equals, int64(100000))
}

func (s *testUtilsSuite) TestGetSQLFileName(c *C) {
	index := &chunk.ChunkID{
		TableIndex:       1,
		BucketIndexLeft:  2,
		BucketIndexRight: 3,
		ChunkIndex:       4,
		ChunkCnt:         10,
	}
	c.Assert(GetSQLFileName(index), Equals, "1:2-3:4")
}

func (s *testUtilsSuite) TestGetChunkIDFromSQLFileName(c *C) {
	tableIndex, bucketIndexLeft, bucketIndexRight, chunkIndex, err := GetChunkIDFromSQLFileName("11:12-13:14")
	c.Assert(err, IsNil)
	c.Assert(tableIndex, Equals, 11)
	c.Assert(bucketIndexLeft, Equals, 12)
	c.Assert(bucketIndexRight, Equals, 13)
	c.Assert(chunkIndex, Equals, 14)
}
