package schema

import (
	"fmt"
	"log"
	"strings"
)

// This struct holds simulated schema states during GenerateIdempotentDDLs().
type Generator struct {
	desiredTables []*Table
	currentTables []*Table
}

// Parse argument DDLs and call `generateDDLs()`
func GenerateIdempotentDDLs(desiredSQL string, currentSQL string) ([]string, error) {
	// TODO: invalidate duplicated tables, columns
	desiredDDLs, err := parseDDLs(desiredSQL)
	if err != nil {
		return nil, err
	}

	currentDDLs, err := parseDDLs(currentSQL)
	if err != nil {
		return nil, err
	}

	tables, err := convertDDLsToTables(currentDDLs)
	if err != nil {
		return nil, err
	}

	generator := Generator{
		desiredTables: []*Table{},
		currentTables: tables,
	}
	return generator.generateDDLs(desiredDDLs)
}

// Main part of DDL genearation
func (g *Generator) generateDDLs(desiredDDLs []DDL) ([]string, error) {
	ddls := []string{}

	// Incrementally examine desiredDDLs
	for _, ddl := range desiredDDLs {
		switch desired := ddl.(type) {
		case *CreateTable:
			if currentTable := findTableByName(g.currentTables, desired.table.name); currentTable != nil {
				// Table already exists, guess required DDLs.
				tableDDLs, err := g.generateDDLsForCreateTable(*currentTable, *desired)
				if err != nil {
					return ddls, err
				}
				ddls = append(ddls, tableDDLs...)
				mergeTable(currentTable, desired.table)
			} else {
				// Table not found, create table.
				ddls = append(ddls, desired.statement)
				table := desired.table // copy table
				g.currentTables = append(g.currentTables, &table)
			}
			table := desired.table // copy table
			g.desiredTables = append(g.desiredTables, &table)
		case *CreateIndex:
			indexDDLs, err := g.generateDDLsForCreateIndex(desired.tableName, desired.index, "CREATE INDEX", ddl.Statement())
			if err != nil {
				return ddls, err
			}
			ddls = append(ddls, indexDDLs...)
		case *AddIndex:
			indexDDLs, err := g.generateDDLsForCreateIndex(desired.tableName, desired.index, "ALTER TABLE", ddl.Statement())
			if err != nil {
				return ddls, err
			}
			ddls = append(ddls, indexDDLs...)
		default:
			return nil, fmt.Errorf("unexpected ddl type in generateDDLs: %v", desired)
		}
	}

	// Clean up obsoleted tables, indexes, columns
	for _, currentTable := range g.currentTables {
		desiredTable := findTableByName(g.desiredTables, currentTable.name)
		if desiredTable == nil {
			// Obsoleted table found. Drop table.
			ddls = append(ddls, fmt.Sprintf("DROP TABLE %s", currentTable.name)) // TODO: escape table name
			g.currentTables = removeTableByName(g.currentTables, currentTable.name)
			continue
		}

		// Table is expected to exist. Check indexes.
		for _, index := range currentTable.indexes {
			if containsString(convertIndexesToIndexNames(desiredTable.indexes), index.name) {
				continue // Index is expected to exist.
			}

			// Index is obsoleted. Check and drop index as needed.
			indexDDLs, err := g.generateDDLsForAbsentIndex(index, *currentTable, *desiredTable)
			if err != nil {
				return ddls, err
			}
			ddls = append(ddls, indexDDLs...)
			// TODO: simulate to remove index from `currentTable.indexes`?
		}

		// Check columns.
		for _, column := range currentTable.columns {
			if containsString(convertColumnsToColumnNames(desiredTable.columns), column.name) {
				continue // Column is expected to exist.
			}

			// Column is obsoleted. Drop column.
			ddl := fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", desiredTable.name, column.name) // TODO: escape
			ddls = append(ddls, ddl)
			// TODO: simulate to remove column from `currentTable.columns`?
		}
	}

	return ddls, nil
}

// In the caller, `mergeTable` manages `g.currentTables`.
func (g *Generator) generateDDLsForCreateTable(currentTable Table, desired CreateTable) ([]string, error) {
	ddls := []string{}

	// Examine each column
	for _, column := range desired.table.columns {
		if containsString(convertColumnsToColumnNames(currentTable.columns), column.name) {
			// TODO: Compare types and change column type!!!
			// TODO: Add unique index if existing column does not have unique flag and there's no unique index!!!!
		} else {
			// Column not found, add column.
			definition, err := g.generateColumnDefinition(column)
			if err != nil {
				return ddls, err
			}
			ddl := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s", desired.table.name, definition) // TODO: escape
			ddls = append(ddls, ddl)
		}
	}

	// Examine each index
	for _, index := range desired.table.indexes {
		if containsString(convertIndexesToIndexNames(currentTable.indexes), index.name) {
			// TODO: Compare types and change column type!!!
		} else {
			// Index not found, add index.
			definition, err := g.generateIndexDefinition(index)
			if err != nil {
				return ddls, err
			}
			ddl := fmt.Sprintf("ALTER TABLE %s ADD %s", desired.table.name, definition) // TODO: escape
			ddls = append(ddls, ddl)
		}
	}

	return ddls, nil
}

// Shared by `CREATE INDEX` and `ALTER TABLE ADD INDEX`.
// This manages `g.currentTables` unlike `generateDDLsForCreateTable`...
func (g *Generator) generateDDLsForCreateIndex(tableName string, desiredIndex Index, action string, statement string) ([]string, error) {
	ddls := []string{}

	currentTable := findTableByName(g.currentTables, tableName)
	if currentTable == nil {
		return nil, fmt.Errorf("%s is performed for inexistent table '%s': '%s'", action, tableName, statement)
	}
	if containsString(convertIndexesToIndexNames(currentTable.indexes), desiredIndex.name) {
		// TODO: compare index definition and change type if necessary
	} else {
		// Index not found, add index.
		ddls = append(ddls, statement)
		currentTable.indexes = append(currentTable.indexes, desiredIndex)
	}

	// Examine indexes in desiredTable to delete obsoleted indexes later
	desiredTable := findTableByName(g.desiredTables, tableName)
	if desiredTable == nil {
		return nil, fmt.Errorf("%s is performed before create table '%s': '%s'", action, tableName, statement)
	}
	if containsString(convertIndexesToIndexNames(desiredTable.indexes), desiredIndex.name) {
		return nil, fmt.Errorf("index '%s' is doubly created against table '%s': '%s'", desiredIndex.name, tableName, statement)
	}
	desiredTable.indexes = append(desiredTable.indexes, desiredIndex)

	return ddls, nil
}

// Even though simulated table doesn't have index, primary or unique could exist in column definitions.
// This carefully generates DROP INDEX for such situations.
func (g *Generator) generateDDLsForAbsentIndex(index Index, currentTable Table, desiredTable Table) ([]string, error) {
	ddls := []string{}

	switch index.indexType {
	case "primary key":
		var primaryKeyColumn *Column
		for _, column := range desiredTable.columns {
			if column.keyOption == ColumnKeyPrimary {
				primaryKeyColumn = &column
				break
			}
		}

		// If nil, it will be `DROP COLUMN`-ed. Ignore it.
		if primaryKeyColumn != nil && primaryKeyColumn.name != index.columns[0].column { // TODO: check length of index.columns
			// TODO: handle this. Rename primary key column...?
			return ddls, fmt.Errorf(
				"primary key column name of '%s' should be '%s' but currently '%s'. This is not handled yet.",
				currentTable.name, primaryKeyColumn.name, index.columns[0].column,
			)
		}
	case "unique key":
		var uniqueKeyColumn *Column
		for _, column := range desiredTable.columns {
			if column.name == index.columns[0].column && (column.keyOption == ColumnKeyUnique || column.keyOption == ColumnKeyUniqueKey) {
				uniqueKeyColumn = &column
				break
			}
		}

		if uniqueKeyColumn == nil {
			// No unique column. Drop unique key index.
			ddl := fmt.Sprintf("ALTER TABLE %s DROP INDEX %s", currentTable.name, index.name) // TODO: escape
			ddls = append(ddls, ddl)
		}
	case "key":
		ddl := fmt.Sprintf("ALTER TABLE %s DROP INDEX %s", currentTable.name, index.name) // TODO: escape
		ddls = append(ddls, ddl)
	default:
		return ddls, fmt.Errorf("unsupported indexType: '%s'", index.indexType)
	}

	return ddls, nil
}

func (g *Generator) generateColumnDefinition(column Column) (string, error) {
	// TODO: make string concatenation faster?
	// TODO: consider escape?

	definition := fmt.Sprintf("%s ", column.name)

	if column.length != nil {
		if column.scale != nil {
			definition += fmt.Sprintf("%s(%s, %s) ", column.typeName, string(column.length.raw), string(column.scale.raw))
		} else {
			definition += fmt.Sprintf("%s(%s) ", column.typeName, string(column.length.raw))
		}
	} else {
		definition += fmt.Sprintf("%s ", column.typeName)
	}

	if column.unsigned {
		definition += "UNSIGNED "
	}
	if column.notNull {
		definition += "NOT NULL "
	}

	if column.defaultVal != nil {
		switch column.defaultVal.valueType {
		case ValueTypeStr:
			definition += fmt.Sprintf("DEFAULT '%s' ", column.defaultVal.strVal)
		case ValueTypeInt:
			definition += fmt.Sprintf("DEFAULT %d ", column.defaultVal.intVal)
		case ValueTypeFloat:
			definition += fmt.Sprintf("DEFAULT %f ", column.defaultVal.floatVal)
		case ValueTypeBit:
			if column.defaultVal.bitVal {
				definition += "DEFAULT b'1' "
			} else {
				definition += "DEFAULT b'0' "
			}
		default:
			return "", fmt.Errorf("unsupported default value type (valueType: '%d') in column: %#v", column.defaultVal.valueType, column)
		}
	}

	if column.autoIncrement {
		definition += "AUTO_INCREMENT "
	}

	switch column.keyOption {
	case ColumnKeyNone:
		// noop
	case ColumnKeyUnique:
		definition += "UNIQUE "
	case ColumnKeyUniqueKey:
		definition += "UNIQUE KEY "
	case ColumnKeyPrimary:
		definition += "PRIMARY KEY "
	default:
		return "", fmt.Errorf("unsupported column key (keyOption: '%d') in column: %#v", column.keyOption, column)
	}

	definition = strings.TrimSuffix(definition, " ")
	return definition, nil
}

func (g *Generator) generateIndexDefinition(index Index) (string, error) {
	definition := index.indexType

	columns := []string{}
	for _, indexColumn := range index.columns {
		columns = append(columns, indexColumn.column)
	}

	definition += fmt.Sprintf(
		" %s(%s)",
		index.name,
		strings.Join(columns, ", "), // TODO: escape
	)
	return definition, nil
}

// Destructively modify table1 to have table2 columns/indexes
func mergeTable(table1 *Table, table2 Table) {
	for _, column := range table2.columns {
		if containsString(convertColumnsToColumnNames(table1.columns), column.name) {
			table1.columns = append(table1.columns, column)
		}
	}

	for _, index := range table2.indexes {
		if containsString(convertIndexesToIndexNames(table1.indexes), index.name) {
			table1.indexes = append(table1.indexes, index)
		}
	}
}

func convertDDLsToTables(ddls []DDL) ([]*Table, error) {
	// TODO: probably "add constraint primary key" support is needed for postgres here.

	tables := []*Table{}
	for _, ddl := range ddls {
		switch ddl := ddl.(type) {
		case *CreateTable:
			table := ddl.table // copy table
			tables = append(tables, &table)
		case *AddIndex:
			// TODO: Add column, etc.
		default:
			return nil, fmt.Errorf("unexpected ddl type in convertDDLsToTables: %v", ddl)
		}
	}
	return tables, nil
}

func findTableByName(tables []*Table, name string) *Table {
	for _, table := range tables {
		if table.name == name {
			return table
		}
	}
	return nil
}

func convertTablesToTableNames(tables []Table) []string {
	tableNames := []string{}
	for _, table := range tables {
		tableNames = append(tableNames, table.name)
	}
	return tableNames
}

func convertColumnsToColumnNames(columns []Column) []string {
	columnNames := []string{}
	for _, column := range columns {
		columnNames = append(columnNames, column.name)
	}
	return columnNames
}

func convertIndexesToIndexNames(indexes []Index) []string {
	indexNames := []string{}
	for _, index := range indexes {
		indexNames = append(indexNames, index.name)
	}
	return indexNames
}

func containsString(strs []string, str string) bool {
	for _, s := range strs {
		if s == str {
			return true
		}
	}
	return false
}

func removeTableByName(tables []*Table, name string) []*Table {
	removed := false
	ret := []*Table{}

	for _, table := range tables {
		if name == table.name {
			removed = true
		} else {
			ret = append(ret, table)
		}
	}

	if !removed {
		log.Fatalf("Failed to removeTableByName: Table `%s` is not found in `%v`", name, tables)
	}
	return ret
}