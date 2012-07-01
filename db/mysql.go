/*
  Copyright (c) 2012 José Carlos Nieto, http://xiam.menteslibres.org/

  Permission is hereby granted, free of charge, to any person obtaining
  a copy of this software and associated documentation files (the
  "Software"), to deal in the Software without restriction, including
  without limitation the rights to use, copy, modify, merge, publish,
  distribute, sublicense, and/or sell copies of the Software, and to
  permit persons to whom the Software is furnished to do so, subject to
  the following conditions:

  The above copyright notice and this permission notice shall be
  included in all copies or substantial portions of the Software.

  THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
  EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
  MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
  NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
  LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
  OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
  WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*/

package db

import (
  "fmt"
  "database/sql"
  "strings"
  "strconv"
  "reflect"
  "regexp"
  _ "code.google.com/p/go-mysql-driver/mysql"
  . "github.com/xiam/gosexy"
)

type Values []string
type Args []string

type myQuery struct {
  Query []string
  Args  []string
}

func myCompile(terms []interface{}) *myQuery {
  q := &myQuery{}

  q.Query = []string{}

  for _, term := range terms {
    switch term.(type) {
      case string: {
        q.Query = append(q.Query, term.(string))
      }
      case Args: {
        for _, arg := range term.(Args) {
          q.Args  = append(q.Args, arg)
        }
      }
      case Values: {
        args := make([]string, len(term.(Values)))
        for i, arg := range term.(Values) {
          args[i] = "?"
          q.Args  = append(q.Args, arg)
        }
        q.Query = append(q.Query, "("+strings.Join(args, ", ")+")")
      }
    }
  }

  return q
}

func myTable(name string) string {
  return name
}

func myFields(names []string) string {
  return "("+strings.Join(names, ", ")+")"
}

func myValues(values []string) Values {
  ret := make(Values, len(values))
  for i, _ := range values {
    ret[i] = values[i]
  }
  return ret
}

type MysqlDB struct {
  config  *DataSource
  session *sql.DB
  collections map[string] Collection
}

func (t *MysqlTable) myFetchAll(rows sql.Rows) []Item {

  items := []Item{}

  columns, _ := rows.Columns()

  for i, _ := range columns {
    columns[i] = strings.ToLower(columns[i])
  }

  res := map[string]*sql.RawBytes{ }

  fargs := []reflect.Value{}

  for _, name := range columns {
    res[name] = &sql.RawBytes{}
    fargs = append(fargs, reflect.ValueOf(res[name]))
  }

  sn := reflect.ValueOf(&rows)
  fn := sn.MethodByName("Scan")

  for rows.Next() {
    item := Item{}

    ret := fn.Call(fargs)

    if ret[0].IsNil() != true {
      panic(ret[1].Elem().Interface().(error))
    }

    for _, name := range columns {
      strval := fmt.Sprintf("%s", *res[name])

      switch t.types[name] {
        case reflect.Uint64: {
          intval, _   := strconv.Atoi(strval)
          item[name]  = uint64(intval)
        }
        case reflect.Int64: {
          intval, _   := strconv.Atoi(strval)
          item[name]  = intval
        }
        case reflect.Float64: {
          floatval, _ := strconv.ParseFloat(strval, 10)
          item[name]  = floatval
        }
        default: {
          item[name]  = strval
        }
      }
    }

    items = append(items, item)
  }

  return items
}

func (my *MysqlDB) myExec(method string, terms ...interface{}) sql.Rows {

  sn := reflect.ValueOf(my.session)
  fn := sn.MethodByName(method)

  q := myCompile(terms)

  fmt.Printf("Q: %v\n", q.Query)
  fmt.Printf("A: %v\n", q.Args)

  args := make([]reflect.Value, len(q.Args) + 1)

  args[0] = reflect.ValueOf(strings.Join(q.Query, " "))

  for i := 0; i < len(q.Args); i++ {
    args[1 + i] = reflect.ValueOf(q.Args[i])
  }

  res := fn.Call(args)

  if res[1].IsNil() == false {
    panic(res[1].Elem().Interface().(error))
  }

  return res[0].Elem().Interface().(sql.Rows)
}

type MysqlTable struct {
  parent *MysqlDB
  name string
  types map[string] reflect.Kind
}

func NewMysqlDB(config *DataSource) Database {
  m := &MysqlDB{}
  m.config = config
  m.collections = make(map[string] Collection)
  return m
}

func (my *MysqlDB) Connect() error {
  var err error

  if my.config.Database == "" {
    panic("Database name is required.")
  }

  my.session, err = sql.Open("mysql", fmt.Sprintf("%s:%s@/%s", my.config.User, my.config.Password, my.config.Database))
  /*
  my.session, err = sql.Open("mysql", fmt.Sprintf("%s:%s@/", my.config.User, my.config.Password))

  if my.config.Database != "" {
    my.Use(my.config.Database)
  }
  */

  return err
}

func (my *MysqlDB) Use(database string) error {
  my.config.Database = database
  my.session.Query(fmt.Sprintf("USE %s", database))
  return nil
}

func (my *MysqlDB) Drop() error {
  my.session.Query(fmt.Sprintf("DROP DATABASE %s", my.config.Database))
  return nil
}

func (my *MysqlDB) Collections() []string {
  var collections []string
  var collection string
  rows, _ := my.session.Query("SHOW TABLES")

  for rows.Next() {
    rows.Scan(&collection)
    collections = append(collections, collection)
  }

  return collections
}

func (t *MysqlTable) invoke(fn string, terms []interface{}) []reflect.Value {

  self    := reflect.ValueOf(t)
  method  := self.MethodByName(fn)

  args := make([]reflect.Value, len(terms))

  itop := len(terms)
  for i := 0; i < itop; i++ {
    args[i] = reflect.ValueOf(terms[i])
  }

  exec := method.Call(args)

  return exec
}

func (t *MysqlTable) compileSet(term Set) (string, Args) {
  sql  := []string{}
  args := Args{}

  for key, arg := range term {
    sql   = append(sql, fmt.Sprintf("%s = ?", key))
    args  = append(args, fmt.Sprintf("%v", arg))
  }

  return strings.Join(sql, ", "), args
}

func (t *MysqlTable) compileConditions(term interface{}) (string, Args) {
  sql  := []string{}
  args := Args{}

  switch term.(type) {
    case []interface{}: {

      itop    := len(term.([]interface{}))

      for i := 0; i < itop; i++ {
        rsql, rargs := t.compileConditions(term.([]interface{})[i])
        if rsql != "" {
          sql   = append(sql, rsql)
          for j := 0; j < len(rargs); j++ {
            args = append(args, rargs[j])
          }
        }
      }

      if len(sql) > 0 {
        return "(" + strings.Join(sql, " AND ") + ")", args
      }
    }
    case Or: {

      itop    := len(term.(Or))

      for i := 0; i < itop; i++ {
        rsql, rargs := t.compileConditions(term.(Or)[i])
        if rsql != "" {
          sql   = append(sql, rsql)
          for j := 0; j < len(rargs); j++ {
            args = append(args, rargs[j])
          }
        }
      }

      if len(sql) > 0 {
        return "(" + strings.Join(sql, " OR ") + ")", args
      }
    }
    case And: {

      itop    := len(term.(Or))

      for i := 0; i < itop; i++ {
        rsql, rargs := t.compileConditions(term.(Or)[i])
        if rsql != "" {
          sql   = append(sql, rsql)
          for j := 0; j < len(rargs); j++ {
            args = append(args, rargs[j])
          }
        }
      }

      if len(sql) > 0 {
        return "(" + strings.Join(sql, " AND ") + ")", args
      }
    }
    case Where: {
      return t.marshal(term.(Where))

    }
  }

  return "", args
}

func (t *MysqlTable) marshal(where Where) (string, []string) {

  for key, val := range(where) {
    key     = strings.Trim(key, " ")
    chunks := strings.Split(key, " ")

    strval := fmt.Sprintf("%v", val)

    if len(chunks) >= 2 {
      return fmt.Sprintf("%s %s ?", chunks[0], chunks[1]), []string{ strval }
    } else {
      return fmt.Sprintf("%s = ?", chunks[0]), []string{ strval }
    }

  }

  return "", []string{ }
}

func (t *MysqlTable) Truncate() bool {

  t.parent.myExec(
    "Query",
    fmt.Sprintf("TRUNCATE TABLE %s", myTable(t.name)),
  )

  return false
}


func (t *MysqlTable) Remove(terms ...interface{}) bool {
  terms = append(terms, Limit(1))

  result := t.invoke("RemoveAll", terms)

  if len(result) > 0 {
    return result[0].Interface().(bool)
  }

  return false
}

func (t *MysqlTable) Update(terms ...interface{}) bool {
  terms = append(terms, Limit(1))

  result := t.invoke("UpdateAll", terms)

  if len(result) > 0 {
    return result[0].Interface().(bool)
  }

  return false
}

func (t *MysqlTable) RemoveAll(terms ...interface{}) bool {
  limit       := ""
  offset      := ""

  conditions, cargs   := t.compileConditions(terms)

  for _, term := range terms {
    switch term.(type) {
      case Limit: {
        limit = fmt.Sprintf("LIMIT %v", term.(Limit))
      }
      case Offset: {
        offset = fmt.Sprintf("OFFSET %v", term.(Offset))
      }
    }
  }

  if conditions == "" {
    conditions = "1 = 1"
  }

  t.parent.myExec(
    "Query",
    fmt.Sprintf("DELETE FROM %s", myTable(t.name)),
    fmt.Sprintf("WHERE %s", conditions), cargs,
    limit, offset,
  )

  return true
}

func (t *MysqlTable) UpdateAll(terms ...interface{}) bool {
  var fields string
  var fargs Args

  limit       := ""
  offset      := ""

  conditions, cargs   := t.compileConditions(terms)

  for _, term := range terms {
    switch term.(type) {
      case Set: {
        fields, fargs = t.compileSet(term.(Set))
      }
      case Limit: {
        limit = fmt.Sprintf("LIMIT %v", term.(Limit))
      }
      case Offset: {
        offset = fmt.Sprintf("OFFSET %v", term.(Offset))
      }
    }
  }

  if conditions == "" {
    conditions = "1 = 1"
  }

  t.parent.myExec(
    "Query",
    fmt.Sprintf("UPDATE %s SET %s", myTable(t.name), fields), fargs,
    fmt.Sprintf("WHERE %s", conditions), cargs,
    limit, offset,
  )

  return true
}

func (t *MysqlTable) FindAll(terms ...interface{}) []Item {
  var itop int

  var relate interface {}
  var relateAll interface {}

  fields      := "*"
  conditions  := ""
  limit       := ""
  offset      := ""


  // Analyzing
  itop = len(terms)

  for i := 0; i < itop; i++ {
    term := terms[i]

    switch term.(type) {
      case Limit: {
        limit = fmt.Sprintf("LIMIT %v", term.(Limit))
      }
      case Offset: {
        offset = fmt.Sprintf("OFFSET %v", term.(Offset))
      }
      case Fields: {
        fields = strings.Join(term.(Fields), ", ")
      }
      case Relate: {
        relate = term.(Relate)
      }
      case RelateAll: {
        relateAll = term.(RelateAll)
      }
    }
  }

  conditions, args := t.compileConditions(terms)

  if conditions == "" {
    conditions = "1 = 1"
  }

  rows := t.parent.myExec(
    "Query",
    fmt.Sprintf("SELECT %s FROM %s", fields, myTable(t.name)),
    fmt.Sprintf("WHERE %s", conditions), args,
    limit, offset,
  )

  result := t.myFetchAll(rows)

  var relations []Tuple
  var rcollection Collection

  // This query is related to other collections.
  if relate != nil {
    for rname, rterms := range relate.(Relate) {

      rcollection = nil

      ttop := len(rterms)
      for t := ttop - 1; t >= 0; t-- {
        rterm := rterms[t]
        switch rterm.(type) {
          case Collection: {
            rcollection = rterm.(Collection)
          }
        }
      }

      if rcollection == nil {
        rcollection = t.parent.Collection(rname)
      }

      relations = append(relations, Tuple { "all": false, "name": rname, "collection": rcollection, "terms": rterms, })
    }
  }

  if relateAll != nil {
    for rname, rterms := range relateAll.(RelateAll) {
      rcollection = nil

      ttop := len(rterms)
      for t := ttop - 1; t >= 0; t-- {
        rterm := rterms[t]
        switch rterm.(type) {
          case Collection: {
            rcollection = rterm.(Collection)
          }
        }
      }

      if rcollection == nil {
        rcollection = t.parent.Collection(rname)
      }

      relations = append(relations, Tuple { "all": true, "name": rname, "collection": rcollection, "terms": rterms, })
    }
  }

  var term interface{}

  jtop := len(relations)

  itop = len(result)
  items := make([]Item, itop)

  for i := 0; i < itop; i++ {

    item := Item{}

    // Default values.
    for key, val := range result[i] {
      item[key] = val
    }

    // Querying relations
    for j := 0; j < jtop; j++ {

      relation := relations[j]

      terms := []interface{}{}

      ktop := len(relation["terms"].(On))

      for k := 0; k < ktop; k++ {

        //term = tcopy[k]
        term = relation["terms"].(On)[k]

        switch term.(type) {
          // Just waiting for Where statements.
          case Where: {
            for wkey, wval := range term.(Where) {
              //if reflect.TypeOf(wval).Kind() == reflect.String { // does not always work.
              if reflect.TypeOf(wval).Name() == "string" {
                // Matching dynamic values.
                matched, _ := regexp.MatchString("\\{.+\\}", wval.(string))
                if matched {
                  // Replacing dynamic values.
                  kname := strings.Trim(wval.(string), "{}")
                  term = Where { wkey : item[kname] }
                }
              }
            }
          }
        }
        terms = append(terms, term)
      }

      // Executing external query.
      if relation["all"] == true {
        value := relation["collection"].(*MysqlTable).invoke("FindAll", terms)
        item[relation["name"].(string)] = value[0].Interface().([]Item)
      } else {
        value := relation["collection"].(*MysqlTable).invoke("Find", terms)
        item[relation["name"].(string)] = value[0].Interface().(Item)
      }

    }

    // Appending to results.
    items[i] = item
  }

  return items
}

func (t *MysqlTable) Count(terms ...interface{}) int {

  terms = append(terms, Fields { "COUNT(1) AS _total" })

  result := t.invoke("FindAll", terms)

  if len(result) > 0 {
    response := result[0].Interface().([]Item)
    if len(response) > 0 {
      val, _ := strconv.Atoi(response[0]["_total"].(string))
      return val
    }
  }

  return 0
}

func (t *MysqlTable) Find(terms ...interface{}) Item {

  var item Item

  terms = append(terms, Limit(1))

  result := t.invoke("FindAll", terms)

  if len(result) > 0 {
    response := result[0].Interface().([]Item)
    if len(response) > 0 {
      item = response[0]
    }
  }

  return item
}

func (t *MysqlTable) Append(items ...interface {}) bool {

  itop := len(items)

  for i := 0; i < itop; i++ {

    values := []string{}
    fields := []string{}

    item := items[i]

    for field, value := range item.(Item) {
      fields = append(fields, field)
      values = append(values, fmt.Sprintf("%v", value))
    }

    t.parent.myExec("Query",
      "INSERT INTO",
      myTable(t.name),
      myFields(fields),
      "VALUES",
      myValues(values),
    )

  }

  return true
}

func (my *MysqlDB) Collection(name string) Collection {

  if collection, ok := my.collections[name]; ok == true {
    return collection
  }

  t := &MysqlTable{}

  t.parent  = my
  t.name    = name

  // Fetching table datatypes and mapping to internal gotypes.

  rows := t.parent.myExec(
    "Query",
    "SHOW COLUMNS FROM", t.name,
  )

  columns := t.myFetchAll(rows)

  pattern, _ := regexp.Compile("^([a-z]+)\\(?([0-9,]+)?\\)?\\s?([a-z]*)?")

  t.types = make(map[string] reflect.Kind, len(columns))

  for _, column := range columns {
    cname := strings.ToLower(column["field"].(string))
    ctype := strings.ToLower(column["type"].(string))
    results := pattern.FindStringSubmatch(ctype)

    // Default properties.
    dextra  := ""
    dtype   := "varchar"

    dtype = results[1]

    if len(results) > 3 {
      dextra = results[3]
    }

    vtype := reflect.String

    // Guessing datatypes.
    switch dtype {
      case "tinyint", "smallint", "mediumint", "int", "bigint": {
        if dextra == "unsigned" {
          vtype = reflect.Uint64
        } else {
          vtype = reflect.Int64
        }
      }
      case "decimal", "float", "double": {
        vtype = reflect.Float64
      }
    }

    /*
    fmt.Printf("Imported %v (from %v)\n", vtype, dtype)
    */

    t.types[cname] = vtype
  }

  my.collections[name] = t

  return t
}


