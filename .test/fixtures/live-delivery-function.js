const payload = JSON.parse(r.body)
payload.functionApplied = "webhookrelay-operator-live-delivery"
r.setBody(JSON.stringify(payload))
r.setHeader("X-WHR-E2E-Function", "applied")
