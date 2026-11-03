(() => {
  const historyKey = 'distributed-job-system.job-history.v1';
  const refreshEveryMs = 3000;
  const jobTypes = {
    addition: { label: 'Addition', operator: '+', help: 'Adds the two numbers and returns the sum.' },
    multiplication: { label: 'Multiplication', operator: '×', help: 'Multiplies the two numbers.' },
    subtraction: { label: 'Subtraction', operator: '−', help: 'Subtracts the second number from the first.' },
    division: { label: 'Integer division', operator: '÷', help: 'Returns the integer quotient. The second number cannot be zero.' },
    fibonacci: { label: 'Fibonacci number', operator: 'index', help: 'Computes Fibonacci(A), where A is from 0 to 46. B is ignored.' },
    prime_count: { label: 'Count primes', operator: '≤', help: 'Counts primes up to A (2 to 10,000,000). B is ignored.' },
    retry_demo: { label: 'Retry demonstration', operator: 'retries', help: 'Fails A times, then completes with B. Set A from 0 to 2 to see retries.' },
    csv_processing: { label: 'CSV Processing', operator: 'count', help: 'Counts exact cell matches in the selected CSV file.' },
    log_analysis: { label: 'Log Analysis', operator: 'analysis', help: 'Counts lines containing ERROR in the selected log file.' },
    image_processing: { label: 'Image Processing', operator: 'analysis', help: 'Validates the selected image file.' },
  };
  const fileJobTypes = new Set(['csv_processing', 'log_analysis', 'image_processing']);
  const arithmeticJobTypes = Object.keys(jobTypes).filter((type) => !fileJobTypes.has(type));
  const fileTypeHelp = {
    csv_processing: 'Counts exact cell matches in the selected CSV file.',
    log_analysis: 'Counts lines containing ERROR in the selected log file.',
    image_processing: 'Validates the selected image file.',
  };
  const records = new Map();
  let refreshing = false;

  const elements = {
    form: document.querySelector('#job-form'),
    type: document.querySelector('#job-type'),
    operator: document.querySelector('#operator'),
    help: document.querySelector('#job-help'),
    formMessage: document.querySelector('#form-message'),
    fileForm: document.querySelector('#file-job-form'),
    fileType: document.querySelector('#file-type'),
    fileTypeHelp: document.querySelector('#file-type-help'),
    s3File: document.querySelector('#s3-file'),
    refreshFiles: document.querySelector('#refresh-files'),
    fileListMessage: document.querySelector('#file-list-message'),
    fileQueryField: document.querySelector('#file-query-field'),
    fileQuery: document.querySelector('#file-query'),
    fileFormMessage: document.querySelector('#file-form-message'),
    randomCount: document.querySelector('#random-count'),
    randomSubmit: document.querySelector('#random-submit'),
    lookupForm: document.querySelector('#lookup-form'),
    lookupID: document.querySelector('#lookup-id'),
    list: document.querySelector('#job-list'),
    empty: document.querySelector('#empty-state'),
    total: document.querySelector('#history-total'),
    updated: document.querySelector('#last-updated'),
  };

  function readHistory() {
    try {
      const value = JSON.parse(localStorage.getItem(historyKey) || '[]');
      return Array.isArray(value) ? value.filter((id) => typeof id === 'string') : [];
    } catch {
      return [];
    }
  }

  let jobIDs = readHistory();

  function saveHistory() {
    try {
      localStorage.setItem(historyKey, JSON.stringify(jobIDs));
    } catch {
      elements.updated.textContent = 'Browser storage is unavailable; history will not persist';
    }
  }

  function rememberJob(id) {
    jobIDs = [id, ...jobIDs.filter((knownID) => knownID !== id)];
    saveHistory();
    render();
    refreshJobs();
  }

  function randomInt(min, max) {
    return Math.floor(Math.random() * (max - min + 1)) + min;
  }

  function randomJobRequest() {
    const types = arithmeticJobTypes;
    const type = types[randomInt(0, types.length - 1)];
    let a = randomInt(-100, 100);
    let b = randomInt(-100, 100);
    if (type === 'division' && b === 0) b = 1;
    if (type === 'fibonacci') a = randomInt(0, 46);
    if (type === 'prime_count') a = randomInt(2, 10000000);
    if (type === 'retry_demo') a = randomInt(0, 2);
    return { type, a, b };
  }

  async function createJob(body) {
    const response = await fetch('/jobs', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
      body: JSON.stringify(body),
    });
    if (!response.ok) {
      const detail = (await response.text()).trim();
      throw new Error(detail || `Request failed (${response.status})`);
    }
    return response.json();
  }

  async function requestJob(id) {
    const response = await fetch(`/jobs/${encodeURIComponent(id)}`, {
      headers: { Accept: 'application/json' },
    });
    if (!response.ok) {
      const detail = (await response.text()).trim();
      throw new Error(detail || `Request failed (${response.status})`);
    }
    return response.json();
  }

  async function requestJobs() {
    const response = await fetch('/jobs', {
      headers: { Accept: 'application/json' },
    });
    if (!response.ok) {
      const detail = (await response.text()).trim();
      throw new Error(detail || `Request failed (${response.status})`);
    }
    const jobs = await response.json();
    if (!Array.isArray(jobs)) throw new Error('Jobs response was malformed');
    return jobs;
  }

  async function requestInputFiles() {
    const response = await fetch('/s3/files', {
      headers: { Accept: 'application/json' },
    });
    if (!response.ok) {
      const detail = (await response.text()).trim();
      throw new Error(detail || `Request failed (${response.status})`);
    }
    const payload = await response.json();
    if (!payload || !Array.isArray(payload.files)) throw new Error('S3 file response was malformed');
    return payload.files.filter((file) => file && typeof file.name === 'string' && typeof file.key === 'string');
  }

  function makeCell(className = '') {
    const cell = document.createElement('td');
    if (className) cell.className = className;
    return cell;
  }

  function renderJob(id) {
    const record = records.get(id) || {};
    const job = record.job;
    const row = document.createElement('tr');

    const idCell = makeCell();
    const idText = document.createElement('span');
    idText.className = 'job-id';
    idText.textContent = job?.ID || id;
    idCell.append(idText);
    if (job?.WorkerID) {
      const worker = document.createElement('span');
      worker.className = 'job-sub';
      worker.textContent = job.WorkerID;
      idCell.append(worker);
    }
    if (record.error) {
      const error = document.createElement('span');
      error.className = 'row-error';
      error.textContent = record.error;
      idCell.append(error);
    }
    row.append(idCell);

    const operationCell = makeCell();
    const operation = document.createElement('span');
    operation.className = 'operation-name';
    operation.textContent = job ? (jobTypes[job.Type]?.label || job.Type) : '—';
    operationCell.append(operation);
    if (job) {
      const expression = document.createElement('span');
      expression.className = 'operation-expression';
      const type = jobTypes[job.Type];
      expression.textContent = type?.operator === 'index' ? `n = ${job.A}`
        : type?.operator === '≤' ? `up to ${job.A}`
          : type?.operator === 'retries' ? `${job.A} planned retries → ${job.B}`
            : `${job.A} ${type?.operator || '?'} ${job.B}`;
      operationCell.append(expression);
      if (fileJobTypes.has(job.Type) && job.Query) {
        const query = document.createElement('span');
        query.className = 'job-sub';
        query.textContent = `query: ${job.Query}`;
        operationCell.append(query);
      }
    }
    row.append(operationCell);

    const statusCell = makeCell();
    const status = job?.Status || (record.error ? 'unavailable' : 'loading');
    const pill = document.createElement('span');
    pill.className = `status-pill status-${['queued', 'processing', 'completed', 'failed'].includes(status) ? status : 'unknown'}`;
    pill.textContent = status;
    statusCell.append(pill);
    if (job?.Status === 'failed') {
      const detail = document.createElement('span');
      detail.className = 'job-sub';
      detail.textContent = 'No failure detail in API';
      statusCell.append(detail);
    }
    row.append(statusCell);

    const resultCell = makeCell();
    const result = document.createElement('span');
    const completed = job?.Status === 'completed';
    result.className = completed ? 'result-value' : 'result-pending';
    result.textContent = completed ? String(job.Result) : '—';
    resultCell.append(result);
    row.append(resultCell);

    const attemptCell = makeCell();
    const attempt = document.createElement('span');
    attempt.className = 'attempt-info';
    attempt.textContent = job ? String(job.Attempt) : '—';
    attemptCell.append(attempt);
    if (job?.Retries > 0) {
      const retries = document.createElement('span');
      retries.className = 'worker-info';
      retries.textContent = `${job.Retries} ${job.Retries === 1 ? 'retry' : 'retries'}`;
      attemptCell.append(retries);
    }
    row.append(attemptCell);

    const inputCell = makeCell();
    const input = document.createElement('span');
    input.className = 'job-sub job-key';
    input.textContent = job?.InputKey || '—';
    input.title = job?.InputKey || '';
    inputCell.append(input);
    row.append(inputCell);

    const outputCell = makeCell();
    const output = document.createElement('span');
    output.className = 'job-sub job-key';
    output.textContent = job?.OutputKey || '—';
    output.title = job?.OutputKey || '';
    outputCell.append(output);
    if (job && fileJobTypes.has(job.Type) && job.Status === 'completed' && job.OutputKey) {
      const download = document.createElement('button');
      download.type = 'button';
      download.className = 'download-button';
      download.textContent = 'Download';
      download.addEventListener('click', () => downloadOutput(id, download));
      outputCell.append(download);
    }
    if (record.downloadError) {
      const error = document.createElement('span');
      error.className = 'row-error';
      error.textContent = record.downloadError;
      outputCell.append(error);
    }
    row.append(outputCell);

    const actionCell = makeCell();
    const remove = document.createElement('button');
    remove.type = 'button';
    remove.className = 'row-action';
    remove.setAttribute('aria-label', `Remove ${id} from history`);
    remove.textContent = '×';
    remove.addEventListener('click', () => {
      jobIDs = jobIDs.filter((knownID) => knownID !== id);
      records.delete(id);
      saveHistory();
      render();
    });
    actionCell.append(remove);
    row.append(actionCell);
    return row;
  }

  function render() {
    elements.list.replaceChildren(...jobIDs.map(renderJob));
    elements.empty.hidden = jobIDs.length > 0;
    elements.total.textContent = String(jobIDs.length);

    const counts = { queued: 0, processing: 0, completed: 0, failed: 0 };
    for (const id of jobIDs) {
      const status = records.get(id)?.job?.Status;
      if (Object.hasOwn(counts, status)) counts[status] += 1;
    }
    document.querySelector('#count-total').textContent = String(jobIDs.length);
    for (const [status, count] of Object.entries(counts)) {
      document.querySelector(`#count-${status}`).textContent = String(count);
    }
  }

  async function refreshJobs() {
    if (refreshing) return;
    refreshing = true;
    try {
      const jobs = await requestJobs();
      const serverIDs = jobs.map((job) => job.ID);
      for (const job of jobs) records.set(job.ID, { job });
      jobIDs = [...serverIDs, ...jobIDs.filter((id) => !serverIDs.includes(id))];
      saveHistory();
    } catch (error) {
      const ids = [...jobIDs];
      await Promise.all(ids.map(async (id) => {
        try {
          records.set(id, { job: await requestJob(id) });
        } catch (lookupError) {
          records.set(id, { ...records.get(id), error: lookupError.message });
        }
      }));
      if (jobIDs.length === 0) elements.updated.textContent = error.message;
    }
    refreshing = false;
    if (jobIDs.length > 0 && records.size > 0) {
      elements.updated.textContent = `Updated ${new Date().toLocaleTimeString()}`;
    }
    render();
  }

  async function refreshInputFiles() {
    elements.refreshFiles.disabled = true;
    elements.fileListMessage.className = 'file-list-message';
    elements.fileListMessage.textContent = 'Loading available files…';
    try {
      const files = await requestInputFiles();
      elements.s3File.replaceChildren();
      if (files.length === 0) {
        elements.s3File.add(new Option('No input files available', ''));
        elements.fileListMessage.textContent = 'No supported files found under input/. Upload an input file before submitting a job.';
        return;
      }
      elements.s3File.add(new Option('Choose an input file…', ''));
      for (const file of files) elements.s3File.add(new Option(file.name, file.key));
      elements.fileListMessage.textContent = `${files.length} file${files.length === 1 ? '' : 's'} available`;
    } catch (error) {
      elements.s3File.replaceChildren(new Option('Unable to load S3 files', ''));
      elements.fileListMessage.className = 'file-list-message error';
      elements.fileListMessage.textContent = error.message;
    } finally {
      elements.refreshFiles.disabled = false;
    }
  }

  async function downloadOutput(id, button) {
    const record = records.get(id) || {};
    button.disabled = true;
    button.textContent = 'Preparing…';
    delete record.downloadError;
    try {
      const response = await fetch(`/jobs/${encodeURIComponent(id)}/download`, {
        headers: { Accept: 'application/json' },
      });
      if (!response.ok) {
        const detail = (await response.text()).trim();
        throw new Error(detail || `Download request failed (${response.status})`);
      }
      const payload = await response.json();
      if (!payload || typeof payload.download_url !== 'string' || !payload.download_url) {
        throw new Error('Download response was malformed');
      }
      window.open(payload.download_url, '_blank', 'noopener,noreferrer');
    } catch (error) {
      record.downloadError = error.message;
      records.set(id, record);
      render();
    } finally {
      button.disabled = false;
      button.textContent = 'Download';
    }
  }

  elements.type.addEventListener('change', () => {
    const type = jobTypes[elements.type.value];
    elements.operator.textContent = type.operator;
    elements.help.textContent = type.help;
  });

  elements.form.addEventListener('submit', async (event) => {
    event.preventDefault();
    const submit = elements.form.querySelector('button[type="submit"]');
    const data = new FormData(elements.form);
    const body = {
      type: String(data.get('type')),
      a: Number(data.get('a')),
      b: Number(data.get('b')),
    };
    submit.disabled = true;
    elements.formMessage.className = 'form-message';
    elements.formMessage.textContent = 'Submitting job…';
    try {
      const job = await createJob(body);
      elements.formMessage.textContent = `Job ${job.ID} submitted`;
      rememberJob(job.ID);
    } catch (error) {
      elements.formMessage.classList.add('error');
      elements.formMessage.textContent = error.message;
    } finally {
      submit.disabled = false;
    }
  });

  elements.fileType.addEventListener('change', () => {
    const isCSV = elements.fileType.value === 'csv_processing';
    elements.fileQueryField.hidden = !isCSV;
    elements.fileQuery.required = isCSV;
    elements.fileTypeHelp.textContent = fileTypeHelp[elements.fileType.value];
  });

  elements.fileForm.addEventListener('submit', async (event) => {
    event.preventDefault();
    const submit = elements.fileForm.querySelector('button[type="submit"]');
    const inputKey = elements.s3File.value;
    const type = elements.fileType.value;
    const query = elements.fileQuery.value.trim();
    if (!inputKey) {
      elements.fileFormMessage.className = 'form-message error';
      elements.fileFormMessage.textContent = 'Choose an S3 input file first.';
      return;
    }
    if (type === 'csv_processing' && !query) {
      elements.fileFormMessage.className = 'form-message error';
      elements.fileFormMessage.textContent = 'Enter an exact CSV cell value to search for.';
      elements.fileQuery.focus();
      return;
    }
    const body = { type, input_key: inputKey };
    if (type === 'csv_processing') body.query = query;
    submit.disabled = true;
    elements.refreshFiles.disabled = true;
    elements.fileFormMessage.className = 'form-message';
    elements.fileFormMessage.textContent = 'Submitting file job…';
    try {
      const job = await createJob(body);
      elements.fileFormMessage.textContent = `Job ${job.ID} submitted`;
      rememberJob(job.ID);
    } catch (error) {
      elements.fileFormMessage.className = 'form-message error';
      elements.fileFormMessage.textContent = error.message;
    } finally {
      submit.disabled = false;
      elements.refreshFiles.disabled = false;
    }
  });

  elements.randomSubmit.addEventListener('click', async () => {
    const count = Number(elements.randomCount.value);
    if (!Number.isSafeInteger(count) || count < 1) {
      elements.randomCount.setCustomValidity('Enter a positive whole number.');
      elements.randomCount.reportValidity();
      return;
    }
    elements.randomCount.setCustomValidity('');
    const submit = elements.form.querySelector('button[type="submit"]');
    submit.disabled = true;
    elements.randomSubmit.disabled = true;
    elements.formMessage.className = 'form-message';
    elements.formMessage.textContent = `Submitting ${count} random jobs…`;
    try {
      const results = await Promise.allSettled(Array.from({ length: count }, () => createJob(randomJobRequest())));
      const created = results.filter((result) => result.status === 'fulfilled').map((result) => result.value);
      for (const job of created) rememberJob(job.ID);
      const failed = results.length - created.length;
      elements.formMessage.textContent = failed
        ? `Submitted ${created.length} of ${count} jobs; ${failed} failed.`
        : `Submitted ${count} random jobs.`;
      if (failed) elements.formMessage.classList.add('error');
    } catch (error) {
      elements.formMessage.classList.add('error');
      elements.formMessage.textContent = error.message;
    } finally {
      submit.disabled = false;
      elements.randomSubmit.disabled = false;
    }
  });

  elements.lookupForm.addEventListener('submit', (event) => {
    event.preventDefault();
    const id = elements.lookupID.value.trim();
    if (id) rememberJob(id);
  });

  document.querySelector('#refresh-button').addEventListener('click', refreshJobs);
  elements.refreshFiles.addEventListener('click', refreshInputFiles);
  document.querySelector('#clear-history').addEventListener('click', () => {
    jobIDs = [];
    records.clear();
    saveHistory();
    elements.updated.textContent = 'History cleared';
    render();
  });

  render();
  elements.fileQueryField.hidden = false;
  elements.fileQuery.required = true;
  refreshInputFiles();
  refreshJobs();
  window.setInterval(refreshJobs, refreshEveryMs);
})();
