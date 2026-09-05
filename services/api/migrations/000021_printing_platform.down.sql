DELETE FROM role_permissions WHERE permission_key IN ('printers.view','printers.manage','printing.print','printing.reprint','print_library.view','print_library.manage');
DELETE FROM permissions WHERE key IN ('printers.view','printers.manage','printing.print','printing.reprint','print_library.view','print_library.manage');
DROP TABLE printer_job_events;
DROP TABLE printer_jobs;
DROP TABLE print_library_assets;
DROP TABLE registered_printers;
DROP TABLE printer_agent_credentials;
DROP TABLE printer_agents;
