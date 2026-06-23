package automation

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func lookupNodeExecutable(t *testing.T) string {
	t.Helper()

	nodePath, err := exec.LookPath("node")
	if err != nil {
		t.Skipf("node is not available: %v", err)
	}

	cmd := exec.Command(nodePath, "-p", "process.execPath")
	output, err := cmd.Output()
	if err != nil {
		return nodePath
	}

	resolved := strings.TrimSpace(string(output))
	if resolved == "" {
		return nodePath
	}
	return resolved
}

func writeMockPlaywrightModule(runtimeDir, version string) error {
	return writeMockPlaywrightModuleWithExpectedEndpoint(runtimeDir, version, "")
}

func writeMockPlaywrightModuleWithExpectedEndpoint(runtimeDir, version, expectedEndpoint string) error {
	return writeMockPlaywrightModuleWithOptions(runtimeDir, version, expectedEndpoint, false)
}

func writeMockPlaywrightModuleWithPersistentConnection(runtimeDir, version, expectedEndpoint string) error {
	return writeMockPlaywrightModuleWithOptions(runtimeDir, version, expectedEndpoint, true)
}

func writeMockPlaywrightModuleCountingNewPages(runtimeDir, version, markerPath string) error {
	moduleDir := filepath.Join(runtimeDir, "node_modules", "playwright-core")
	if err := os.MkdirAll(moduleDir, 0o755); err != nil {
		return err
	}

	packageJSON := fmt.Sprintf("{\"name\":\"playwright-core\",\"version\":\"%s\",\"main\":\"index.js\"}", version)
	if err := os.WriteFile(filepath.Join(moduleDir, "package.json"), []byte(packageJSON), 0o644); err != nil {
		return err
	}

	markerPathJSON, err := json.Marshal(markerPath)
	if err != nil {
		return err
	}

	indexJS := fmt.Sprintf(`const fs = require('fs');

const markerPath = %s;
let newPageCount = 0;

const context = {
  async newPage() {
    newPageCount += 1;
    fs.writeFileSync(markerPath, String(newPageCount));
    return {
      async bringToFront() {},
      async goto() {},
      async waitForLoadState() {},
      async waitForTimeout() {},
      isClosed() {
        return false;
      },
      url() {
        return 'about:blank';
      },
    };
  },
  pages() {
    return [];
  },
};

exports.chromium = {
  async connectOverCDP() {
    return {
      contexts() {
        return [context];
      },
      async close() {},
    };
  },
};
`, string(markerPathJSON))
	return os.WriteFile(filepath.Join(moduleDir, "index.js"), []byte(indexJS), 0o644)
}

func writeMockPlaywrightModuleWithExpectedConnectTimeout(runtimeDir, version string, expectedConnectTimeout int) error {
	moduleDir := filepath.Join(runtimeDir, "node_modules", "playwright-core")
	if err := os.MkdirAll(moduleDir, 0o755); err != nil {
		return err
	}

	packageJSON := fmt.Sprintf("{\"name\":\"playwright-core\",\"version\":\"%s\",\"main\":\"index.js\"}", version)
	if err := os.WriteFile(filepath.Join(moduleDir, "package.json"), []byte(packageJSON), 0o644); err != nil {
		return err
	}

	indexJS := fmt.Sprintf(`const expectedConnectTimeout = %d;

const context = {
  async grantPermissions() {},
  async newPage() {
    return {
      async goto() {},
      async bringToFront() {},
      async waitForLoadState() {},
      async waitForTimeout() {},
      async close() {},
      isClosed() {
        return false;
      },
      async title() {
        return 'Mock Page Title';
      },
      url() {
        return 'about:blank';
      },
    };
  },
  pages() {
    return [];
  },
};

exports.chromium = {
  async connectOverCDP(endpoint, options = {}) {
    if (options.timeout !== expectedConnectTimeout) {
      throw new Error('unexpected connect timeout: ' + String(options.timeout));
    }
    return {
      contexts() {
        return [context];
      },
      async close() {},
    };
  },
};
`, expectedConnectTimeout)
	return os.WriteFile(filepath.Join(moduleDir, "index.js"), []byte(indexJS), 0o644)
}

func writeMockPlaywrightModuleWithOptions(runtimeDir, version, expectedEndpoint string, persistentConnection bool) error {
	moduleDir := filepath.Join(runtimeDir, "node_modules", "playwright-core")
	if err := os.MkdirAll(moduleDir, 0o755); err != nil {
		return err
	}

	packageJSON := fmt.Sprintf("{\"name\":\"playwright-core\",\"version\":\"%s\",\"main\":\"index.js\"}", version)
	if err := os.WriteFile(filepath.Join(moduleDir, "package.json"), []byte(packageJSON), 0o644); err != nil {
		return err
	}

	expectedEndpointJSON, err := json.Marshal(expectedEndpoint)
	if err != nil {
		return err
	}
	persistentConnectionJSON, err := json.Marshal(persistentConnection)
	if err != nil {
		return err
	}

	indexJS := fmt.Sprintf(`const fs = require('fs');

const expectedEndpoint = %s;
const persistentConnection = %s;

function createPage() {
  let currentURL = 'about:blank';
  return {
    async goto(url) {
      currentURL = url;
    },
    async bringToFront() {},
    async waitForLoadState() {},
    async waitForTimeout() {},
    async screenshot(options) {
      fs.writeFileSync(options.path, 'mock-screenshot');
    },
    async evaluate(fn, arg) {
      const previousFetch = global.fetch;
      global.fetch = async (url, init = {}) => {
        return {
          ok: String(init.method || 'GET').toUpperCase() !== 'DELETE',
          status: String(init.method || 'GET').toUpperCase() === 'POST' ? 201 : 200,
          statusText: String(init.method || 'GET').toUpperCase() === 'DELETE' ? 'Forbidden' : 'OK',
          url: String(url),
          headers: {
            forEach(callback) {
              callback('application/json', 'content-type');
            },
          },
          async text() {
            return JSON.stringify({
              ok: true,
              url: String(url),
              method: String(init.method || 'GET').toUpperCase(),
              credentials: init.credentials || '',
              headers: init.headers || {},
              body: init.body || '',
            });
          },
        };
      };
      try {
        return await fn(arg);
      } finally {
        global.fetch = previousFetch;
      }
    },
    async title() {
      return 'Mock Page Title';
    },
    url() {
      return currentURL;
    },
    isClosed() {
      return false;
    },
    async close() {},
  };
}

const initialPage = createPage();

const context = {
  async grantPermissions() {},
  async newPage() {
    return createPage();
  },
  pages() {
    return [initialPage];
  },
};

exports.chromium = {
  async connectOverCDP(endpoint) {
    if (String(endpoint).includes(':0')) {
      throw new Error('invalid cdp endpoint');
    }
    if (expectedEndpoint && endpoint !== expectedEndpoint) {
      throw new Error('unexpected cdp endpoint: ' + endpoint);
    }
    const hold = persistentConnection ? setInterval(() => {}, 1000) : null;
    return {
      contexts() {
        return [context];
      },
      async close() {
        if (hold) {
          clearInterval(hold);
        }
      },
    };
  },
};
`, string(expectedEndpointJSON), string(persistentConnectionJSON))
	return os.WriteFile(filepath.Join(moduleDir, "index.js"), []byte(indexJS), 0o644)
}
