const API_BASE_URL = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";

export type Job = { id:string; status:"queued"|"processing"|"needs_review"|"processed"|"failed"; parser_version:string; total_pages:number; processed_pages:number; created_at:string; updated_at:string };
export type JobDetails = { job:Job; orders:Array<{id:string;source_page:number;marketplace_order_id:string|null;awb:string|null;status:string;items:Array<{raw_sku:string|null;product_id:string|null;quantity:number|null;quantity_source:string;resolution_status:string;warnings:string[]}>}>;errors:Array<{source_page:number|null;severity:string;code:string;message:string}> };

async function parse<T>(response:Response):Promise<T>{if(!response.ok){const body=await response.json().catch(()=>({})) as {error?:{message?:string}};throw new Error(body.error?.message??`Request failed (${response.status})`)}return await response.json() as T;}
export const flipkartAPI={
  upload:async(file:File)=>{const data=new FormData();data.append("file",file);return parse<{job:Job;duplicate_source:boolean}>(await fetch(`${API_BASE_URL}/api/v1/flipkart/jobs`,{method:"POST",credentials:"include",body:data}));},
  job:async(id:string)=>parse<JobDetails>(await fetch(`${API_BASE_URL}/api/v1/flipkart/jobs/${id}`,{credentials:"include"})),
  retry:async(id:string)=>parse<{job:Job}>(await fetch(`${API_BASE_URL}/api/v1/flipkart/jobs/${id}`,{method:"POST",credentials:"include"})),
};
