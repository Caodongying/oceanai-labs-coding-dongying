1. **A service stores user names in users.display_name. A proposed release introduces
users.full_name, changes all application reads and writes to the new column, and drops
display_name in the same deployment. The service uses rolling deployment, so old and new
application instances run concurrently. The release must also support rollback. Explain why
this release can fail and propose a safe sequence of database and application changes.**

As the old instance is still running and using users.display_name. Dropping this field  will lead to issues like "field not found". To safely migrate to the new database schema and ensure compatibility‌, we should keep both users.full_name and users.display_name. If one of them need to be updated, update the other as well(it would be good to do the double write in a transaction). When the old instances are shut down or we're sure that the old release is not needed anymore, we can drop the old field.

2. **Certificate details are cached for 60 seconds. Request A misses the cache and reads ACTIVE
from the database. Before A writes to the cache, request B revokes the certificate, commits
the database transaction, and deletes the cache entry. A then writes its stale ACTIVE value
into the cache. A colleague proposes reducing the cache TTL to five seconds. Explain why this
does not fix the problem. Redesign the read and revocation paths so that any read starting
after B returns success cannot receive the stale ACTIVE value.**

The colleague's proposal doesn't work since if the same request flow completes in less than five seconds, the issue still occurs. The conflict itself has nothing to do with the cache TTL. I first thought if database outbox can be adopted here, a.k.a putting database and cache operation in one transaction, write this transaction record to database, and start a separate worker to handle the outbox records. Then I realized that this method is too bad due to low performance. And if there're multiple workers, the issue will occur again.

The easiest way is to remove the certificate details from cache and always read from database :)

Inspired by fencing token, I then realized that adding an increasing version number in the certificate can solve this problem. Request A reads both version number e.g. 1001 and ACTIVE status from the database. The cache key A looks for might be certificate:Axxx:1001. When request B revokes the certificate, it also updates the version number to 1002. The following requests will first retrieve the latest version number and use it to get/update certificate details in the cache. The old cache key certificate:Axxx:1001 is no longer used.

3. Service A has 50 request workers. Every request to its /report endpoint calls Service B with a 100 ms timeout and up to three retries. Traffic to /report remains at 1,000 requests per
second. B's latency suddenly rises from 20 ms to 2 seconds, and unrelated endpoints on A
soon stop responding. Explain the chain of failure, including the effect of retries. Redesign
/report so unrelated endpoints remain available, and state what /report should return when
B is unhealthy.

As the traffic to /report is 1000 requests per second, each worker is supposed to process 20 requests per second on average. However when Service B is down or experiences high latency, each request to /report can take up to 400ms, meaning each worker is able to handle 2.5 requests at most on average. All 50 workers waits for B's response, so requests to other endpoints cannot be processed due to the lack of free workers. Service A is therefore down as well.

The endpoint /report should use a circuit breaker so that it can quickly return error code like 503 service unavailable when Service B is down.
