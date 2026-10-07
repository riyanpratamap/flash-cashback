// TanStack Query flushes its batched updates from a timer, outside any act() scope, so a query that settles
// after a test's last await updates React unwrapped and React warns. Wrap each flush in a synchronous act()
// where the test environment supports it (files that render through Testing Library).
const { notifyManager } = require('@tanstack/react-query');
const { act } = require('react');

notifyManager.setNotifyFunction((fn) => {
  if (globalThis.IS_REACT_ACT_ENVIRONMENT) act(fn);
  else fn();
});
