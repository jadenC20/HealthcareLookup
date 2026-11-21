import { createWorker } from 'tesseract.js';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
const here=path.dirname(fileURLToPath(import.meta.url));
const buffers=[];
for await (const chunk of process.stdin) buffers.push(chunk);
const worker=await createWorker('eng',1,{langPath:path.join(here,'ocr-data'),cacheMethod:'none',gzip:true});
try { const result=await worker.recognize(Buffer.concat(buffers)); process.stdout.write(result.data.text); }
finally { await worker.terminate(); }
