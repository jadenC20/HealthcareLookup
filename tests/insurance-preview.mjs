// Optional synthetic browser fixture. Never uses OpenAI or a real account.
// Run: node tests/insurance-preview.mjs, then visit http://127.0.0.1:4184/#account
import http from 'node:http';
import {createFrontendServer} from '../server.mjs';
import {result as fixture} from './fixtures/insurance.mjs';
let result={...fixture,caution:'Synthetic UI test response — not a real insurance plan.'};
const backend=http.createServer(async(req,res)=>{
 res.setHeader('Content-Type','application/json');
 const send=value=>res.end(JSON.stringify(value));
 if(req.url==='/health')return send({status:'ok'});
 if(req.url==='/auth/me')return send({user:{id:'synthetic',name:'Interface test',city:'',plan:'Synthetic plan'},csrf:'synthetic-token'});
 if(req.url==='/profile/insurance-card')return send({has_card:true,updated:fixture.updated,lookup_status:'partial'});
 if(req.url==='/profile/insurance')return send({revision:'synthetic-revision',result});
 if(req.url==='/reviews')return send({results:[]});
 if(req.method==='POST'&&['/profile/insurance/retry','/profile/insurance/select','/profile/insurance/sbc','/profile/insurance-card'].includes(req.url)||req.method==='PUT'&&req.url==='/profile/insurance/details'){
  for await(const chunk of req){};
  result={...fixture,status:'processing'};
  setTimeout(()=>{result={...fixture,caution:'Synthetic UI test response — not a real insurance plan.',status:'partial'};send({revision:'synthetic-revision',result});},1000);return;
 }
 res.statusCode=404;send({detail:'This synthetic fixture does not implement that action.'});
});
backend.listen(4185,'127.0.0.1');
createFrontendServer({backendUrl:'http://127.0.0.1:4185'}).listen(4184,'127.0.0.1',()=>console.log('Synthetic insurance UI: http://127.0.0.1:4184/#account'));
