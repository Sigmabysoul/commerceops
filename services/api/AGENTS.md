This is the main Go backend.

Domain logic belongs under internal/<domain>.
HTTP handlers should remain thin.
Database access must respect tenant isolation.