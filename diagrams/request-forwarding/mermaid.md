# Request Forwarding

```
title Key Write Flow with Internal Consistent Hashing
autoNumber nested
// Actors
Client [icon: azure-administrative-units, color: blue]
Ingress Node [icon: azure-support-center-blue, color: orange]
Owner [icon: azure-lock, color: green]
// Client initiates a write request to any node in the cluster
Client > Ingress Node: HTTP /kv/k v
activate Ingress Node
// Ingress Node checks if it is the owner of the key using internal consistent hashing
Ingress Node > Ingress Node: Calculate hash of key 'k'
Ingress Node > Ingress Node: Determine owner node for key
// Decision: Is Ingress Node the owner?
alt [label: Ingress Node is Owner, icon: user-check, color: blue] {
  // Ingress Node processes the write directly
  Ingress Node > Ingress Node: Process operation for key 'k'

  Ingress Node --> Client: 200 OK
  Ingress Node --> Client: 500 Internal Server Error
}
else [label: Ingress Node is NOT Owner, icon: user-x, color: orange] {
  // Ingress Node forwards the write to the correct owner node
  Ingress Node > Owner: Forward request for key 'k'
  activate Owner

    Owner --> Ingress Node: OK
    Ingress Node --> Client: 200 OK
    Owner --> Ingress Node: Error (e.g., network partition)
    Ingress Node --> Client: 500 Internal Server Error
  deactivate Owner
}
deactivate Ingress Nodeo
```
