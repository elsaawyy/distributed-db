// Package shared documents the cross-node data contracts.
// This package is for documentation reference only —
// each node (master, worker-go, worker-python) maintains its own copy
// of these models to remain independently deployable.

/*
Shared Data Contracts
=====================

ReplicationPayload fields by operation:

  create_db:
    operation: "create_db"
    database: "<name>"

  drop_db:
    operation: "drop_db"
    database: "<name>"

  create_table:
    operation: "create_table"
    database: "<name>"
    table: "<table_name>"
    schema:
      name: "<table_name>"
      columns: [{name, type, required}]
      created_at: "<ISO8601>"

  insert:
    operation: "insert"
    database: "<name>"
    table: "<table_name>"
    record:
      id: "<unique_id>"
      fields: {<col>: <val>}
      created_at: "<ISO8601>"
      updated_at: "<ISO8601>"

  update:
    operation: "update"
    database: "<name>"
    table: "<table_name>"
    record_id: "<id>"
    fields: {<col>: <val>}

  delete:
    operation: "delete"
    database: "<name>"
    table: "<table_name>"
    record_id: "<id>"
*/
package shared
