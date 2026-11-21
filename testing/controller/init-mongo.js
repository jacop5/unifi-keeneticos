// Executed only when the dedicated test database volume is first created.
db = db.getSiblingDB('admin');
db.createUser({
  user: process.env.MONGO_USER,
  pwd: process.env.MONGO_PASS,
  roles: [
    'clusterMonitor',
    ...['unifi', 'unifi_stat', 'unifi_audit', 'unifi_restore'].map(
      name => ({ role: 'dbOwner', db: name })
    ),
  ],
});
