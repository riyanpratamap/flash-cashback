// Tests run with the device in UTC, so a day built through `Date` would be wrong for WIB data (AC-66).
module.exports = async () => {
  process.env.TZ = 'UTC';
};
