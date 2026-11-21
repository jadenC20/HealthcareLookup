import {mkdir,writeFile} from 'node:fs/promises';
const directory=new URL('./ocr-data/',import.meta.url);
await mkdir(directory,{recursive:true});
const response=await fetch('https://tessdata.projectnaptha.com/4.0.0/eng.traineddata.gz');
if(!response.ok)throw new Error('Unable to download English OCR model');
await writeFile(new URL('eng.traineddata.gz',directory),Buffer.from(await response.arrayBuffer()));
console.log('English OCR model ready locally.');
