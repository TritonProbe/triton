async function load(id, path) {
  const target = document.getElementById(id);
  try {
    const response = await fetch(path);
    const data = await response.json();
    target.textContent = JSON.stringify(data, null, 2);
  } catch (error) {
    target.textContent = String(error);
  }
}

load("status", "/api/v1/status");
load("probes", "/api/v1/probes");
load("benches", "/api/v1/benches");
