# Bully election algorithm

## Schema

- /metrics (GET)
- /health (GET)
- /election (POST)
- /leader (POST)

## Service Discovery

- list pods from namespace
  - use labels
- get service endpoints in a namespace, then use them to list pods
  - use labels

Use case: network partitioning. Several clusters in one namespace. Labels can help to separate pods into several partitions.

## Problems

Calculate how many requests failed in each broadcast.

Set timeout for each state. Pod must leave state if they stuck.

There is a deadlock somewhere!
